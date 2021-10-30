/*
 * This file is part of the boiler-mate distribution (https://github.com/mlipscombe/boiler-mate).
 * Copyright (c) 2021 Mark Lipscombe.
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
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

func randomString(len int) (string, error) {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		nBig, err := rand.Int(rand.Reader, big.NewInt(25))
		if err != nil {
			return "", err
		}
		bytes[i] = byte(65 + nBig.Int64()) //A=65 and Z = 65+25
	}
	return string(bytes), nil
}

type NBE struct {
	URI          *url.URL
	AppID        string
	ControllerID string
	Serial       string
	SeqNo        int8
	PinCode      string
	RSAKey       *rsa.PublicKey // rsa key

	SettingSchema map[string]SettingDefinition
	Ready         chan bool

	listener   net.PacketConn
	queue      map[int8]func(*NBEResponse)
	queueMutex sync.RWMutex
}

func NewNBE(uri *url.URL) (*NBE, error) {
	appID, err := randomString(12)
	if err != nil {
		return nil, err
	}
	controllerID, err := randomString(6)
	if err != nil {
		return nil, err
	}
	password, _ := uri.User.Password()
	nbe := NBE{
		URI:          uri,
		AppID:        appID,
		ControllerID: controllerID,
		Serial:       uri.User.Username(),
		PinCode:      password,
		SeqNo:        0,
		Ready:        make(chan bool),
		queue:        make(map[int8]func(*NBEResponse)),
		queueMutex:   sync.RWMutex{},
	}
	err = nbe.connect()
	return &nbe, err
}

func (nbe *NBE) listen() chan error {
	// doneChan := make(chan error, 1)
	defer nbe.listener.Close()

	for {
		buffer := make([]byte, 1024)

		_, addr, err := nbe.listener.ReadFrom(buffer)
		if addr.String() != nbe.URI.Host {
			// ignore packets from other hosts
			continue
		}
		if err != nil {
			log.Errorln(err)
		}
		go nbe.handle(buffer)
	}

	// return doneChan
}

func (nbe *NBE) handle(buffer []byte) {
	var response NBEResponse
	reader := bytes.NewReader(buffer)
	err := response.Unpack(reader)
	if err != nil {
		log.Errorf("failed to unpack response: %s", err)
		return
	}

	log.Debugf("recv %d %d %s", response.SeqNo, response.Function, response.Payload)

	if response.SeqNo == -1 {
		// Probably an error packet, log the payload.
		log.Errorf("protocol error: %s", response.Payload["error"])
		return
	}

	nbe.queueMutex.RLock()
	if val, ok := nbe.queue[response.SeqNo]; ok {
		nbe.queueMutex.RUnlock()
		val(&response)
		nbe.queueMutex.Lock()
		delete(nbe.queue, response.SeqNo)
		nbe.queueMutex.Unlock()
	} else {
		nbe.queueMutex.RUnlock()
		log.Infof("sequence %d has no callback", response.SeqNo)
	}
}

func (nbe *NBE) connect() error {
	listener, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	nbe.listener = listener

	go nbe.listen()

	request := NBERequest{
		AppID:        nbe.AppID,
		ControllerID: nbe.ControllerID,
		Function:     DiscoveryFunction,
		Payload:      []byte("NBE Discovery"),
	}

	response, err := nbe.Send(&request)
	if err != nil {
		return err
	}
	nbe.Serial = fmt.Sprintf("%v", response.Payload["serial"])
	pub, err := nbe.getRSAKey()
	if err != nil {
		return err
	}
	nbe.RSAKey = pub

	return nil
}

func (nbe *NBE) SendAsync(request *NBERequest, cb func(*NBEResponse)) (int8, error) {
	var err error

	nbe.queueMutex.Lock()
	nbe.SeqNo++
	if nbe.SeqNo > 99 {
		nbe.SeqNo = 0
	}
	nbe.queueMutex.Unlock()

	request.SeqNo = nbe.SeqNo

	addr, err := net.ResolveUDPAddr("udp4", nbe.URI.Host)
	if err != nil {
		return request.SeqNo, err
	}
	packet := new(bytes.Buffer)
	err = request.Pack(packet)
	if err != nil {
		return request.SeqNo, err
	}

	nbe.queueMutex.Lock()
	nbe.queue[request.SeqNo] = cb
	nbe.queueMutex.Unlock()

	log.Debugf("send %d %d %s", request.SeqNo, request.Function, request.Payload)

	_, err = nbe.listener.WriteTo(packet.Bytes(), addr)
	if err != nil {
		nbe.queueMutex.Lock()
		delete(nbe.queue, request.SeqNo)
		nbe.queueMutex.Unlock()

		return request.SeqNo, err
	}

	return request.SeqNo, nil
}

func (nbe *NBE) Send(request *NBERequest) (*NBEResponse, error) {
	responseChan := make(chan *NBEResponse, 1)

	_, err := nbe.SendAsync(request, func(response *NBEResponse) {
		responseChan <- response
	})

	if err != nil {
		return nil, err
	}

	select {
	case response := <-responseChan:
		return response, nil
	case <-time.After(time.Duration(3) * time.Second):
		return nil, errors.New("timeout waiting for request")
	}
}

func (nbe *NBE) GetAsync(function Function, path string, cb func(*NBEResponse)) (int8, error) {
	request := NBERequest{
		AppID:        nbe.AppID,
		ControllerID: nbe.ControllerID,
		Function:     function,
		Payload:      []byte(path),
	}
	seq, err := nbe.SendAsync(&request, cb)

	return seq, err
}

func (nbe *NBE) Get(function Function, path string) (*NBEResponse, error) {
	request := NBERequest{
		AppID:        nbe.AppID,
		ControllerID: nbe.ControllerID,
		Function:     function,
		Payload:      []byte(path),
	}

	return nbe.Send(&request)
}

func (nbe *NBE) SetAsync(path string, value []byte, cb func(*NBEResponse)) (int8, error) {
	payload := new(bytes.Buffer)
	payload.Write([]byte(path))
	payload.Write([]byte("="))
	payload.Write(value)

	request := NBERequest{
		AppID:        nbe.AppID,
		ControllerID: nbe.ControllerID,
		Function:     SetSetupFunction,
		RSAKey:       nbe.RSAKey,
		PinCode:      nbe.PinCode,
		Payload:      payload.Bytes(),
	}
	seq, err := nbe.SendAsync(&request, cb)

	return seq, err
}

func (nbe *NBE) Set(path string, value []byte) (*NBEResponse, error) {
	payload := new(bytes.Buffer)
	payload.Write([]byte(path))
	payload.Write([]byte("="))
	payload.Write(value)

	request := NBERequest{
		AppID:        nbe.AppID,
		ControllerID: nbe.ControllerID,
		Function:     SetSetupFunction,
		RSAKey:       nbe.RSAKey,
		PinCode:      nbe.PinCode,
		Payload:      payload.Bytes(),
	}

	return nbe.Send(&request)
}

func (nbe *NBE) getRSAKey() (*rsa.PublicKey, error) {
	if nbe.RSAKey != nil {
		return nbe.RSAKey, nil
	}

	response, err := nbe.Get(GetSetupFunction, "misc.rsa_key")
	if err != nil {
		return nil, err
	}

	pub, err := rsaKeyFromBase64(response.Payload["rsa_key"].(string))
	if err != nil {
		return nil, err
	}
	return pub, nil
}

func rsaKeyFromBase64(key string) (*rsa.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return nil, err
	}
	pub, err := x509.ParsePKIXPublicKey(b)
	if err != nil {
		return nil, err
	}
	return pub.(*rsa.PublicKey), nil
}
