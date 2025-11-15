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
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"time"
)

// Use shared constants from frame_helpers.go

type NBERequest struct {
	AppID        string         // client application id
	ControllerID string         // controller id
	RSAKey       *rsa.PublicKey // rsa key
	Function     Function
	SeqNo        int8
	PinCode      string
	Timestamp    time.Time
	Payload      []byte
}

func (frame *NBERequest) Validate() error {
	if len(frame.AppID) == 0 {
		return fmt.Errorf("AppID is empty")
	}
	if len(frame.ControllerID) == 0 {
		return fmt.Errorf("ControllerID is empty")
	}
	if len(frame.Payload) == 0 {
		return fmt.Errorf(" Payload is empty")
	}
	return nil
}

func (frame *NBERequest) Pack(writer io.Writer) error {
	var err error
	err = frame.Validate()
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, []byte(fmt.Sprintf("%12s", frame.AppID)))
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, []byte(fmt.Sprintf("%06s", frame.ControllerID)))
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	err = buf.WriteByte(0x02)
	if err != nil {
		return err
	}
	_, err = buf.WriteString(fmt.Sprintf("%02d", frame.Function))
	if err != nil {
		return err
	}
	_, err = buf.WriteString(fmt.Sprintf("%02d", frame.SeqNo))
	if err != nil {
		return err
	}
	if frame.PinCode != "" {
		_, err = buf.WriteString(fmt.Sprintf("%10s", frame.PinCode))
	} else {
		_, err = buf.WriteString("0000000000")
	}
	if err != nil {
		return err
	}
	if frame.Timestamp.IsZero() {
		frame.Timestamp = time.Now()
	}
	_, err = buf.WriteString(fmt.Sprintf("%010d", frame.Timestamp.Unix()))
	if err != nil {
		return err
	}
	_, err = buf.WriteString("extr")
	if err != nil {
		return err
	}
	_, err = buf.WriteString(fmt.Sprintf("%03d", len(frame.Payload)))
	if err != nil {
		return err
	}
	_, err = buf.Write(frame.Payload)
	if err != nil {
		return err
	}
	err = buf.WriteByte(0x04)
	if err != nil {
		return err
	}

	if frame.RSAKey != nil {
		padLen := 64 - buf.Len()
		padBytes := make([]byte, padLen)
		_, err = rand.Read(padBytes)
		if err != nil {
			return err
		}

		binary.Write(writer, binary.BigEndian, []byte("*"))
		err = binary.Write(buf, binary.BigEndian, padBytes)
		if err != nil {
			return err
		}
		c := new(big.Int).SetBytes(buf.Bytes())
		err = binary.Write(writer, binary.BigEndian, c.Exp(c, big.NewInt(int64(frame.RSAKey.E)), frame.RSAKey.N).Bytes())
		if err != nil {
			return err
		}
	} else {
		binary.Write(writer, binary.BigEndian, []byte(" "))
		_, err = buf.WriteTo(writer)
		if err != nil {
			return err
		}
	}
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

	// Skip encryption byte and validate start marker
	if err := readAndValidateMarker(reader, StartMarker, "start marker"); err != nil {
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

	// Read and ignore end marker
	if _, err = readBytes(reader, 1, "end marker"); err != nil {
		return err
	}

	return nil
}

// Helper functions are now in frame_helpers.go
