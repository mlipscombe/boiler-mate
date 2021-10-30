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
	var err error
	err = binary.Read(reader, binary.BigEndian, frame.AppID[:12])
	if err != nil {
		return err
	}
	err = binary.Read(reader, binary.BigEndian, frame.ControllerID[:6])
	if err != nil {
		return err
	}
	encryption := make([]byte, 1)
	err = binary.Read(reader, binary.BigEndian, encryption)
	if err != nil {
		return err
	}
	startMarker := make([]byte, 1)
	err = binary.Read(reader, binary.BigEndian, startMarker)
	if err != nil {
		return err
	}
	if startMarker[0] != 0x02 {
		return fmt.Errorf("invalid start marker: %c", startMarker[0])
	}
	err = binary.Read(reader, binary.BigEndian, &frame.Function)
	if err != nil {
		return err
	}
	err = binary.Read(reader, binary.BigEndian, &frame.SeqNo)
	if err != nil {
		return err
	}
	err = binary.Read(reader, binary.BigEndian, frame.PinCode[:10])
	if err != nil {
		return err
	}
	var ts int64
	err = binary.Read(reader, binary.BigEndian, &ts)
	if err != nil {
		return err
	}
	frame.Timestamp = time.Unix(ts, 0)
	err = binary.Read(reader, binary.BigEndian, "extr")
	if err != nil {
		return err
	}
	var payloadLenBytes [3]byte
	err = binary.Read(reader, binary.BigEndian, &payloadLenBytes)
	if err != nil {
		return err
	}
	payloadLen := int(payloadLenBytes[0]) | int(payloadLenBytes[1])<<8 | int(payloadLenBytes[2])<<16
	frame.Payload = make([]byte, int(payloadLen))
	err = binary.Read(reader, binary.BigEndian, frame.Payload)
	if err != nil {
		return err
	}
	err = binary.Read(reader, binary.BigEndian, 0x04)
	if err != nil {
		return err
	}

	return nil
}
