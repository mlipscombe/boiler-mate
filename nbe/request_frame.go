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
	"fmt"
	"io"
	"math/big"
	"time"
)

const encryptedBodySize = 64

type NBERequest struct {
	AppID        string
	ControllerID string
	RSAKey       *rsa.PublicKey
	Function     Function
	SeqNo        int8
	PinCode      string
	Timestamp    time.Time
	Payload      []byte
}

func (frame *NBERequest) Validate() error {
	if len(frame.AppID) == 0 || len(frame.AppID) > AppIDSize ||
		len(frame.ControllerID) == 0 || len(frame.ControllerID) > ControllerIDSize {
		return fmt.Errorf("invalid request identifiers")
	}
	if frame.Function < 0 || frame.Function > 99 || frame.SeqNo < 0 || frame.SeqNo > 99 {
		return fmt.Errorf("invalid request function or sequence")
	}
	if len(frame.PinCode) > PinCodeSize || len(frame.Payload) == 0 || len(frame.Payload) > 999 {
		return fmt.Errorf("invalid controller PIN or request payload length")
	}
	const bodyOverhead = 2 + FunctionSize + SeqNoSize + PinCodeSize + TimestampSize + ExtrMarkerSize + PayloadLenSize
	if frame.RSAKey != nil && len(frame.Payload) > encryptedBodySize-bodyOverhead {
		return fmt.Errorf("request payload exceeds the NBE encrypted frame limit")
	}
	return nil
}

func (frame *NBERequest) Pack(writer io.Writer) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	timestamp := frame.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	if seconds := timestamp.Unix(); seconds < 0 || seconds > 9999999999 {
		return fmt.Errorf("invalid request timestamp")
	}
	pin := frame.PinCode
	if pin == "" {
		pin = "0000000000"
	}

	var packet bytes.Buffer
	fmt.Fprintf(&packet, "%12s%6s", frame.AppID, frame.ControllerID)

	body := fmt.Appendf(nil, "%c%02d%02d%10s%010dextr%03d",
		StartMarker, frame.Function, frame.SeqNo, pin, timestamp.Unix(), len(frame.Payload))
	body = append(body, frame.Payload...)
	body = append(body, EndMarker)
	if key := frame.RSAKey; key != nil {
		if key.N == nil || key.N.Sign() <= 0 || key.E < 3 || key.E%2 == 0 {
			return fmt.Errorf("invalid controller RSA key")
		}
		var padded [encryptedBodySize]byte
		copy(padded[:], body)
		_, _ = rand.Read(padded[len(body):])
		plain := new(big.Int).SetBytes(padded[:])
		if plain.Cmp(key.N) >= 0 {
			return fmt.Errorf("controller RSA key is too small for the NBE frame")
		}
		// NBE expects the raw RSA operation on its own padded frame.
		packet.WriteByte('*')
		ciphertext := plain.Exp(plain, big.NewInt(int64(key.E)), key.N)
		packet.Write(ciphertext.FillBytes(make([]byte, key.Size())))
	} else {
		packet.WriteByte(' ')
		packet.Write(body)
	}
	_, err := packet.WriteTo(writer)
	return err
}

func (frame *NBERequest) Unpack(reader io.Reader) error {
	// Read fixed-size string fields
	var err error
	if frame.AppID, err = readString(reader, AppIDSize, "AppID"); err != nil {
		return err
	}
	if frame.ControllerID, err = readString(reader, ControllerIDSize, "ControllerID"); err != nil {
		return err
	}

	// Encrypted fixture requests are decrypted before unpacking.
	if err := validateMarker(reader, ' ', "encryption marker"); err != nil {
		return err
	}
	if err := validateMarker(reader, StartMarker, "start marker"); err != nil {
		return err
	}

	// Read function and sequence number (ASCII encoded)
	if frame.Function, err = readASCIIInt16(reader, FunctionSize, "Function"); err != nil {
		return err
	}
	if frame.SeqNo, err = readASCIIInt8(reader, SeqNoSize, "SeqNo"); err != nil {
		return err
	}

	// Read PinCode
	if frame.PinCode, err = readString(reader, PinCodeSize, "PinCode"); err != nil {
		return err
	}

	// Read timestamp (ASCII encoded Unix timestamp)
	var ts int64
	if ts, err = readASCIIInt64(reader, TimestampSize, "Timestamp"); err != nil {
		return err
	}
	frame.Timestamp = time.Unix(ts, 0)

	// Skip "extr" marker
	if _, err = readBytes(reader, ExtrMarkerSize, "extr marker"); err != nil {
		return err
	}

	// Read payload length and payload
	var payloadLen int
	if payloadLen, err = readASCIIInt(reader, PayloadLenSize, "payload length"); err != nil {
		return err
	}

	if frame.Payload, err = readBytes(reader, payloadLen, "payload"); err != nil {
		return err
	}

	return validateMarker(reader, EndMarker, "end marker")
}
