package nbe

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestRequestSequenceWrapAndQuarantine(t *testing.T) {
	client := &NBE{next: 99}
	first, err := client.reserve(&pendingRequest{})
	if err != nil || first != 99 {
		t.Fatalf("first sequence = %d, %v", first, err)
	}
	client.reuseAt[0] = time.Now().Add(time.Minute)
	for want := 1; want < 99; want++ {
		seq, err := client.reserve(&pendingRequest{})
		if err != nil || seq != want {
			t.Fatalf("sequence = %d, %v, want %d", seq, err, want)
		}
	}
	if _, err := client.reserve(&pendingRequest{}); err == nil {
		t.Fatal("reserved an occupied or quarantined sequence")
	}
	client.reuseAt[0] = time.Time{}
	if seq, err := client.reserve(&pendingRequest{}); err != nil || seq != 0 {
		t.Fatalf("sequence after quarantine = %d, %v, want 0", seq, err)
	}
}

func TestControllerReplyMatchingAndCancellation(t *testing.T) {
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	client := &NBE{
		appID: "ABCDEFGHIJKL", controllerID: "ABCDEF",
		peer: peer.LocalAddr().(*net.UDPAddr).AddrPort(),
		conn: socket, done: make(chan struct{}),
	}
	go client.listen()
	t.Cleanup(client.Close)

	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	seen := make(chan struct{})
	errorsFromPeer := make(chan error, 1)
	go func() {
		errorsFromPeer <- respondAfterCancellation(peer, seen)
	}()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	firstResult := make(chan error, 1)
	go func() {
		_, err := client.Get(ctx, GetSetupFunction, "boiler.temp")
		firstResult <- err
	}()
	select {
	case <-seen:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("controller did not receive the first read")
	}
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("read error = %v", err)
	}
	response, err := client.Get(t.Context(), GetSetupFunction, "boiler.diff_over")
	if err != nil || response.Payload["diff_over"] != int64(10) {
		t.Fatalf("matching response = %v, %v", response, err)
	}
	if err := <-errorsFromPeer; err != nil {
		t.Fatal(err)
	}
}

func respondAfterCancellation(peer *net.UDPConn, seen chan<- struct{}) error {
	var buf [1024]byte
	n, _, err := peer.ReadFromUDPAddrPort(buf[:])
	if err != nil {
		return err
	}
	var first NBERequest
	if err := first.Unpack(bytes.NewReader(buf[:n])); err != nil {
		return err
	}
	close(seen)
	n, addr, err := peer.ReadFromUDPAddrPort(buf[:])
	if err != nil {
		return err
	}
	var second NBERequest
	if err := second.Unpack(bytes.NewReader(buf[:n])); err != nil {
		return err
	}
	return sendReplies(peer, addr, first, second)
}

func sendReplies(peer *net.UDPConn, to netip.AddrPort, previous, current NBERequest) error {
	malformed := []byte{StartMarker}
	if _, err := peer.WriteToUDPAddrPort(malformed, to); err != nil {
		return err
	}
	for _, response := range []NBEResponse{
		{SeqNo: previous.SeqNo, Function: previous.Function, Payload: map[string]any{"temp": 73}},
		{SeqNo: current.SeqNo, Function: GetOperatingDataFunction},
		{SeqNo: current.SeqNo, Function: current.Function, Payload: map[string]any{"diff_over": 10}},
	} {
		var buffer bytes.Buffer
		if err := response.Pack(&buffer); err != nil {
			return err
		}
		if _, err := peer.WriteToUDPAddrPort(buffer.Bytes(), to); err != nil {
			return err
		}
	}
	return nil
}

func TestInvalidControllerFrames(t *testing.T) {
	// Negative payload lengths used to panic in the UDP packet handler.
	packet := []byte("ABCDEFGHIJKLABCDEF\x0201000-01\x04")
	var response NBEResponse
	if err := response.Unpack(bytes.NewReader(packet)); err == nil {
		t.Fatal("accepted an invalid response length")
	}
	if _, err := rsaKeyFromBase64(base64.StdEncoding.EncodeToString([]byte("invalid DER"))); err == nil {
		t.Fatal("accepted malformed RSA key")
	}
	request := NBERequest{AppID: "ABCDEFGHIJKL", ControllerID: "ABCDEF", Function: SetSetupFunction, RSAKey: &rsa.PublicKey{}, Payload: bytes.Repeat([]byte{'x'}, 100)}
	if err := request.Pack(io.Discard); err == nil {
		t.Fatal("accepted an oversized encrypted frame")
	}
}
