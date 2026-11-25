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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MockBoiler simulates an NBE boiler for testing
type MockBoiler struct {
	writeAttempts atomic.Int64
	Serial        string
	Port          int
	listener      net.PacketConn
	running       bool         // Protected by mu
	mu            sync.RWMutex // Protects running and data
	data          map[string]map[string]interface{}
	rawReplies    map[Function]mockReply // Protected by mu
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
		rawReplies:    make(map[Function]mockReply),
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
		_ = mb.listener.Close()
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

	_, err = mb.listener.WriteTo(responseBuffer.Bytes(), addr)
	if err != nil {
		// Log error but don't fail - this is a mock server
		return
	}
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

	mb.mu.RLock()
	fixed, ok := mb.rawReplies[request.Function]
	mb.mu.RUnlock()
	if ok {
		response.Status, response.RawPayload = fixed.status, fixed.payload
		return response
	}

	switch request.Function {
	case DiscoveryFunction:
		response.Payload["serial"] = mb.Serial
		response.Payload["rsa_key"] = mb.rsaKeyBase64

	case GetSetupFunction:
		path := string(request.Payload)
		response.Payload = mb.getData(path)

	case GetSetupRangeFunction:
		switch string(request.Payload) {
		case "boiler.*":
			response.Payload["temp"] = "40,85,65,1"
			response.Payload["diff_under"] = "0,50,10,0"
			response.Payload["diff_over"] = "10,20,12,0"
		default:
			response.Status = 1
		}

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
		mb.writeAttempts.Add(1)
		response.Status = 1
		response.Payload["error"] = "settings writes are disabled in the simulator"

	// The replies below follow the layouts read from the NBE app. Their
	// contents are invented: no real controller reply has been captured.
	case CustomRequestFunction:
		response.Status, response.RawPayload = mb.customReply(string(request.Payload))

	case GetEventLogFunction:
		if _, err := time.Parse("060102-150405", string(request.Payload)); err != nil {
			response.Status = 1
			break
		}
		response.RawPayload = "241124,101500,0,12,60,65,;241124,093000,4,13,0,0,;241123,220000,6,0,0,0,;"

	case GetChartDataFunction:
		response.Status, response.RawPayload = mockChartReply(string(request.Payload))

	case GetAvailableProgramsFunction:
		if string(request.Payload) != AvailableProgramsRequest {
			response.Status = 1
			break
		}
		response.RawPayload = "150203,0616,0;150410,0702,1;"

	case GetRemoteSensorsFunction:
		if string(request.Payload) != RemoteSensorsRequest {
			response.Status = 1
			break
		}
		response.RawPayload = "00:11:22:33:44:55,21.5,3.1,3600,42,0,128;66:77:88:99:AA:BB,19.0,2.9,3600,300;"

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

type mockReply struct {
	status  uint8
	payload string
}

// SetRawReply makes the mock answer a function with a fixed status and
// payload, so tests can exercise rejected and malformed replies.
func (mb *MockBoiler) SetRawReply(function Function, status uint8, payload string) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.rawReplies[function] = mockReply{status: status, payload: payload}
}

// customReply answers function 11 from the operating and advanced data. An
// unknown name is rejected; what a real controller does with one is unknown.
func (mb *MockBoiler) customReply(payload string) (uint8, string) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	var values []string
	for name := range strings.SplitSeq(payload, ",") {
		value, ok := mb.data["operating"][name]
		if !ok {
			value, ok = mb.data["advanced"][name]
		}
		if !ok {
			return 1, ""
		}
		values = append(values, fmt.Sprint(value))
	}
	return 0, strings.Join(values, ",")
}

// mockChartReply answers function 7 with three samples per request: a clock
// time, then the two relative forms the app accepts.
func mockChartReply(payload string) (uint8, string) {
	request, ok := strings.CutSuffix(payload, ";")
	fields := strings.Split(request, ",")
	if !ok || len(fields) < 4 {
		return 1, ""
	}
	for index, layout := range []string{"060102", "150405", "150405"} {
		if _, err := time.Parse(layout, fields[index]); err != nil {
			return 1, ""
		}
	}
	ids := fields[3:]
	if strings.HasPrefix(ids[0], "step=") {
		ids = ids[1:]
	}
	if len(ids) == 0 {
		return 1, ""
	}
	var reply strings.Builder
	for row, stamp := range []string{fields[1], "60", "time=60"} {
		reply.WriteString(stamp)
		for _, field := range ids {
			id, err := strconv.Atoi(field)
			if err != nil {
				return 1, ""
			}
			fmt.Fprintf(&reply, ",%.1f", float64(id)+float64(row)/2)
		}
		reply.WriteByte(';')
	}
	return 0, reply.String()
}

// WriteAttempts reports how many write requests reached the read-only test controller.
func (mb *MockBoiler) WriteAttempts() int64 {
	return mb.writeAttempts.Load()
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
