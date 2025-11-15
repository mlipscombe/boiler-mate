/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021-2023 Mark Lipscombe.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, version 3.
 *
 * This program is distributed in the hope that it will be useful, but
 * WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
 * General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

package nbe

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
)

// MockBoiler simulates an NBE boiler for testing
type MockBoiler struct {
	Serial        string
	Port          int
	listener      net.PacketConn
	running       bool              // Protected by mu
	mu            sync.RWMutex      // Protects running and data
	data          map[string]map[string]interface{}
	rsaPrivateKey *rsa.PrivateKey
	rsaPublicKey  *rsa.PublicKey
	rsaKeyBase64  string
}

// NewMockBoiler creates a new mock boiler server
func NewMockBoiler(serial string) (*MockBoiler, error) {
	// Generate RSA key for mock boiler
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	rsaKeyBase64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	mb := &MockBoiler{
		Serial:        serial,
		rsaPrivateKey: privateKey,
		rsaPublicKey:  &privateKey.PublicKey,
		rsaKeyBase64:  rsaKeyBase64,
		data:          make(map[string]map[string]interface{}),
	}

	// Initialize mock data
	mb.initializeData()

	return mb, nil
}

// Start begins listening for UDP packets
func (mb *MockBoiler) Start() error {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	mb.listener = listener
	mb.Port = listener.LocalAddr().(*net.UDPAddr).Port
	
	mb.mu.Lock()
	mb.running = true
	mb.mu.Unlock()

	go mb.listen()
	return nil
}

// Stop shuts down the mock boiler
func (mb *MockBoiler) Stop() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.running = false
	if mb.listener != nil {
		mb.listener.Close()
	}
}

// GetAddr returns the address string for connecting
func (mb *MockBoiler) GetAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", mb.Port)
}

func (mb *MockBoiler) listen() {
	for {
		mb.mu.RLock()
		running := mb.running
		mb.mu.RUnlock()
		
		if !running {
			return
		}
		
		buffer := make([]byte, 1024)
		n, addr, err := mb.listener.ReadFrom(buffer)
		if err != nil {
			mb.mu.RLock()
			stillRunning := mb.running
			mb.mu.RUnlock()
			if stillRunning {
				continue
			}
			return
		}
		go mb.handleRequest(buffer[:n], addr)
	}
}

func (mb *MockBoiler) handleRequest(data []byte, addr net.Addr) {
	// Ignore empty or too-small packets
	if len(data) < 20 {
		return
	}

	// Check if this is an encrypted request (starts with "*")
	// Format: AppID(12) + ControllerID(6) + Encryption marker(1) + encrypted data
	if len(data) > 19 && data[18] == '*' {
		// This is an RSA-encrypted request
		// Extract the encrypted data (everything after the encryption marker)
		encryptedData := data[19:]

		// Decrypt using modular exponentiation: c^d mod n
		// This matches the encryption: c^e mod n
		c := new(big.Int).SetBytes(encryptedData)
		d := mb.rsaPrivateKey.D
		n := mb.rsaPrivateKey.N

		// Perform decryption: m = c^d mod n
		decrypted := new(big.Int).Exp(c, d, n)
		decryptedData := decrypted.Bytes()

		// The decrypted data should be exactly 64 bytes (padded)
		// Remove padding to get the actual payload
		// The payload is at the beginning, padding is at the end
		if len(decryptedData) > 64 {
			// If longer than 64 bytes, something went wrong
			return
		}

		// Ensure it's 64 bytes by prepending zeros if needed
		if len(decryptedData) < 64 {
			padded := make([]byte, 64)
			copy(padded[64-len(decryptedData):], decryptedData)
			decryptedData = padded
		}

		// Reconstruct the packet: AppID + ControllerID + " " + decrypted payload
		reconstructed := make([]byte, 0, 19+len(decryptedData))
		reconstructed = append(reconstructed, data[0:18]...) // AppID + ControllerID
		reconstructed = append(reconstructed, ' ')           // Space instead of *
		reconstructed = append(reconstructed, decryptedData...)
		data = reconstructed
	}

	var request NBERequest
	reader := bytes.NewReader(data)
	err := request.Unpack(reader)
	if err != nil {
		// Silently ignore malformed requests in mock
		return
	}

	response := mb.processRequest(&request)
	responseBuffer := new(bytes.Buffer)
	err = response.Pack(responseBuffer)
	if err != nil {
		return
	}

	mb.listener.WriteTo(responseBuffer.Bytes(), addr)
}

func (mb *MockBoiler) processRequest(request *NBERequest) *NBEResponse {
	response := &NBEResponse{
		AppID:        request.AppID,
		ControllerID: request.ControllerID,
		Function:     request.Function,
		SeqNo:        request.SeqNo,
		Status:       0,
		Payload:      make(map[string]interface{}),
	}

	switch request.Function {
	case DiscoveryFunction:
		response.Payload["serial"] = mb.Serial
		response.Payload["rsa_key"] = mb.rsaKeyBase64

	case GetSetupFunction:
		path := string(request.Payload)
		response.Payload = mb.getData(path)

	case GetOperatingDataFunction:
		mb.mu.RLock()
		if data, ok := mb.data["operating"]; ok {
			response.Payload = copyMap(data)
		}
		mb.mu.RUnlock()

	case GetAdvancedDataFunction:
		mb.mu.RLock()
		if data, ok := mb.data["advanced"]; ok {
			response.Payload = copyMap(data)
		}
		mb.mu.RUnlock()

	case SetSetupFunction:
		// Parse key=value from payload
		payload := string(request.Payload)
		parts := strings.SplitN(payload, "=", 2)
		if len(parts) == 2 {
			mb.setData(parts[0], parts[1])
			response.Payload["status"] = "ok"
		}

	default:
		response.Payload["error"] = "unsupported function"
	}

	return response
}

func (mb *MockBoiler) getData(path string) map[string]interface{} {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	result := make(map[string]interface{})

	if path == "*" || strings.HasSuffix(path, ".*") {
		// Get all data for a category
		category := strings.TrimSuffix(path, ".*")
		if category == "*" {
			// Return all data
			for k, v := range mb.data {
				for ik, iv := range v {
					result[fmt.Sprintf("%s.%s", k, ik)] = iv
				}
			}
		} else if data, ok := mb.data[category]; ok {
			result = copyMap(data)
		}
	} else {
		// Get specific key
		parts := strings.SplitN(path, ".", 2)
		if len(parts) == 2 {
			category := parts[0]
			key := parts[1]
			if data, ok := mb.data[category]; ok {
				if val, ok := data[key]; ok {
					result[key] = val
				}
			}
		}
	}

	return result
}

func (mb *MockBoiler) setData(path, value string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 2 {
		category := parts[0]
		key := parts[1]
		if _, ok := mb.data[category]; !ok {
			mb.data[category] = make(map[string]interface{})
		}
		mb.data[category][key] = value
	}
}

// SetValue allows tests to set mock data
func (mb *MockBoiler) SetValue(category, key string, value interface{}) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	if _, ok := mb.data[category]; !ok {
		mb.data[category] = make(map[string]interface{})
	}
	mb.data[category][key] = value
}

// GetValue allows tests to get mock data
func (mb *MockBoiler) GetValue(category, key string) (interface{}, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if data, ok := mb.data[category]; ok {
		val, ok := data[key]
		return val, ok
	}
	return nil, false
}

func (mb *MockBoiler) initializeData() {
	// Initialize misc settings
	mb.data["misc"] = map[string]interface{}{
		"rsa_key": mb.rsaKeyBase64,
	}

	// Initialize boiler settings
	mb.data["boiler"] = map[string]interface{}{
		"temp":       RoundedFloat(65.0),
		"diff_under": RoundedFloat(5.0),
		"diff_over":  RoundedFloat(15.0),
	}

	// Initialize regulation settings
	mb.data["regulation"] = map[string]interface{}{
		"boiler_power_min": int64(30),
		"boiler_power_max": int64(100),
	}

	// Initialize oxygen settings
	mb.data["oxygen"] = map[string]interface{}{
		"start_calibrate": int64(0),
	}

	// Initialize hopper settings
	mb.data["hopper"] = map[string]interface{}{
		"content": RoundedFloat(150.0),
	}

	// Initialize operating data
	mb.data["operating"] = map[string]interface{}{
		"boiler_temp": RoundedFloat(62.5),
		"smoke_temp":  RoundedFloat(125.3),
		"oxygen":      RoundedFloat(12.5),
		"power_kw":    RoundedFloat(15.2),
		"power_pct":   RoundedFloat(75.0),
		"photo_level": RoundedFloat(88.0),
		"state":       int64(5), // Power state
		"state_text":  PowerStates[5],
	}

	// Initialize advanced data
	mb.data["advanced"] = map[string]interface{}{
		"fan_speed":    int64(2500),
		"auger_cycles": int64(120),
	}
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{})
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
