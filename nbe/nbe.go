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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	requestTimeout = 3 * time.Second
	sequenceCount  = 100
)

// NBE is a UDP client for a single controller.
type NBE struct {
	Serial    string
	IPAddress string

	appID        string
	controllerID string
	pinCode      string
	rsaKey       *rsa.PublicKey

	conn *net.UDPConn
	peer netip.AddrPort
	done chan struct{}

	mu      sync.Mutex
	next    int
	pending [sequenceCount]*pendingRequest
	reuseAt [sequenceCount]time.Time
	readErr error
}

type pendingRequest struct {
	function Function
	response chan *NBEResponse
}

// Open discovers the controller and loads its RSA key on the same UDP socket
// used for subsequent polling and writes.
func Open(ctx context.Context, uri *url.URL) (*NBE, error) {
	if uri == nil || uri.Hostname() == "" || uri.Port() == "" {
		return nil, errors.New("controller hostname and port are required")
	}

	addr, err := resolveController(ctx, uri)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, fmt.Errorf("open controller socket: %w", err)
	}

	pin, _ := uri.User.Password()
	client := &NBE{
		IPAddress:    uri.Hostname(),
		appID:        randomID(AppIDSize),
		controllerID: randomID(ControllerIDSize),
		pinCode:      pin,
		conn:         conn,
		peer:         addr,
		done:         make(chan struct{}),
		next:         1,
	}
	go client.listen()

	if err := client.discover(ctx); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func resolveController(ctx context.Context, uri *url.URL) (netip.AddrPort, error) {
	port, err := strconv.ParseUint(uri.Port(), 10, 16)
	if err != nil || port == 0 {
		return netip.AddrPort{}, errors.New("invalid controller port")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", uri.Hostname())
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("resolve controller host: %w", err)
	}
	if len(ips) == 0 {
		return netip.AddrPort{}, errors.New("controller host has no IPv4 address")
	}
	return netip.AddrPortFrom(ips[0].Unmap(), uint16(port)), nil
}

func randomID(size int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	id := make([]byte, 0, size)
	// Reject bytes that would bias the distribution over the alphabet.
	var sample [32]byte
	for len(id) < size {
		_, _ = rand.Read(sample[:])
		for _, b := range sample {
			if b < 234 {
				id = append(id, alphabet[int(b)%len(alphabet)])
				if len(id) == size {
					break
				}
			}
		}
	}
	return string(id)
}

func (client *NBE) discover(ctx context.Context) error {
	response, err := client.Get(ctx, DiscoveryFunction, "NBE Discovery")
	if err != nil {
		return fmt.Errorf("discover controller: %w", err)
	}
	switch serial := response.Payload["serial"].(type) {
	case string:
		client.Serial = serial
	case int64:
		client.Serial = strconv.FormatInt(serial, 10)
	}
	if client.Serial == "" || strings.ContainsAny(client.Serial, "/+#\x00") {
		return errors.New("controller returned an invalid serial number")
	}

	response, err = client.Get(ctx, GetSetupFunction, "misc.rsa_key")
	if err != nil {
		return fmt.Errorf("read controller RSA key: %w", err)
	}
	encoded, ok := response.Payload["rsa_key"].(string)
	if !ok || encoded == "" {
		return errors.New("controller response is missing its RSA key")
	}
	client.rsaKey, err = rsaKeyFromBase64(encoded)
	return err
}

func (client *NBE) Close() {
	_ = client.conn.Close()
	<-client.done
}

func (client *NBE) listen() {
	defer close(client.done)
	const maxFrameSize = AppIDSize + ControllerIDSize + FunctionSize + SeqNoSize + StatusSize + PayloadLenSize + 999 + 2
	buffer := make([]byte, maxFrameSize+1)
	for {
		n, sender, err := client.conn.ReadFromUDPAddrPort(buffer)
		if err != nil {
			client.mu.Lock()
			client.readErr = fmt.Errorf("read controller response: %w", err)
			client.mu.Unlock()
			return
		}
		if sender != client.peer || n > maxFrameSize {
			continue
		}
		var response NBEResponse
		reader := bytes.NewReader(buffer[:n])
		if err := response.Unpack(reader); err != nil || response.SeqNo < 0 || reader.Len() != 0 {
			continue
		}
		client.mu.Lock()
		request := client.pending[response.SeqNo]
		if request != nil && request.function == response.Function {
			select {
			case request.response <- &response:
			default:
			}
		}
		client.mu.Unlock()
	}
}

func (client *NBE) reserve(request *pendingRequest) (int, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.readErr != nil {
		return 0, client.readErr
	}
	now := time.Now()
	for range sequenceCount {
		seq := client.next
		client.next = (client.next + 1) % sequenceCount
		if client.pending[seq] == nil && !now.Before(client.reuseAt[seq]) {
			client.pending[seq] = request
			return seq, nil
		}
	}
	return 0, errors.New("controller request sequence numbers are busy")
}

// ResponseError reports a controller rejection without including its payload.
type ResponseError struct {
	Function Function
	Status   uint8
}

func (err *ResponseError) Error() string {
	return fmt.Sprintf("controller rejected function %d with status %d", err.Function, err.Status)
}

func (client *NBE) send(ctx context.Context, request NBERequest) (*NBEResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	pending := &pendingRequest{function: request.Function, response: make(chan *NBEResponse, 1)}
	seq, err := client.reserve(pending)
	if err != nil {
		return nil, err
	}
	sent := false
	defer func() {
		client.mu.Lock()
		client.pending[seq] = nil
		if sent {
			// Give late replies time to drain before the wire ID is reused.
			client.reuseAt[seq] = time.Now().Add(requestTimeout)
		}
		client.mu.Unlock()
	}()

	request.AppID = client.appID
	request.ControllerID = client.controllerID
	request.SeqNo = int8(seq)
	var packet bytes.Buffer
	if err := request.Pack(&packet); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := client.conn.WriteToUDPAddrPort(packet.Bytes(), client.peer); err != nil {
		return nil, fmt.Errorf("send controller request: %w", err)
	}
	sent = true

	select {
	case response := <-pending.response:
		if response.Status != 0 {
			return nil, &ResponseError{Function: response.Function, Status: response.Status}
		}
		return response, nil
	case <-client.done:
		client.mu.Lock()
		defer client.mu.Unlock()
		return nil, client.readErr
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (client *NBE) Get(ctx context.Context, function Function, path string) (*NBEResponse, error) {
	return client.send(ctx, NBERequest{Function: function, Payload: []byte(path)})
}

func (client *NBE) Set(ctx context.Context, path string, value []byte) (*NBEResponse, error) {
	if client.pinCode == "" {
		return nil, errors.New("a controller password is required for settings writes")
	}
	return client.send(ctx, NBERequest{
		Function: SetSetupFunction,
		RSAKey:   client.rsaKey,
		PinCode:  client.pinCode,
		Payload:  append([]byte(path+"="), value...),
	})
}

func rsaKeyFromBase64(encoded string) (*rsa.PublicKey, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid base64 controller RSA key")
	}
	publicKey, err := x509.ParsePKIXPublicKey(key)
	if err != nil {
		return nil, errors.New("invalid controller public key encoding")
	}
	rsaKey, ok := publicKey.(*rsa.PublicKey)
	if !ok || rsaKey.N == nil || rsaKey.N.Sign() <= 0 || rsaKey.E < 3 || rsaKey.E%2 == 0 {
		return nil, errors.New("controller public key is not a valid RSA key")
	}
	return rsaKey, nil
}
