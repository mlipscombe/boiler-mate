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
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type NBEResponse struct {
	AppID        string // client application id
	ControllerID string // controller id
	Function     Function
	SeqNo        int8
	Status       uint8
	Payload      map[string]interface{}
}

func (frame *NBEResponse) Pack(writer io.Writer) error {
	var err error

	err = binary.Write(writer, binary.BigEndian, []byte(fmt.Sprintf("%12s", frame.AppID)))
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, []byte(fmt.Sprintf("%6s", frame.ControllerID)))
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, [1]byte{0x02})
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, &frame.Function)
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, &frame.SeqNo)
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, &frame.Status)
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, &frame.Payload)
	if err != nil {
		return err
	}
	err = binary.Write(writer, binary.BigEndian, [1]byte{0x04})
	if err != nil {
		return err
	}

	return nil
}

func (frame *NBEResponse) Unpack(reader io.Reader) error {
	var err error

	appId := make([]byte, 12)
	if _, err = io.ReadFull(reader, appId); err != nil {
		return err
	}
	frame.AppID = string(appId)

	controllerId := make([]byte, 6)
	if _, err = io.ReadFull(reader, controllerId); err != nil {
		return err
	}
	frame.ControllerID = string(controllerId)

	startMarker := make([]byte, 1)
	if _, err = io.ReadFull(reader, startMarker); err != nil {
		return err
	}
	if startMarker[0] != 0x02 {
		return fmt.Errorf("invalid start marker: %x", startMarker[0])
	}

	function := make([]byte, 2)
	if _, err = io.ReadFull(reader, function); err != nil {
		return err
	}
	functionInt, err := strconv.ParseInt(strings.TrimSpace(string(function)), 10, 8)
	if err != nil {
		functionInt = -1
	}
	frame.Function = Function(functionInt)

	seqNo := make([]byte, 2)
	if _, err = io.ReadFull(reader, seqNo); err != nil {
		return fmt.Errorf("invalid seq no: %s", string(seqNo))
	}
	seqNoInt, err := strconv.ParseInt(strings.TrimSpace(string(seqNo)), 10, 8)
	if err != nil {
		seqNoInt = -1
	}
	frame.SeqNo = int8(seqNoInt)

	status := make([]byte, 1)
	if _, err = io.ReadFull(reader, status); err != nil {
		return err
	}
	statusInt, err := strconv.ParseUint(strings.TrimSpace(string(status)), 10, 8)
	if err != nil {
		return fmt.Errorf("invalid status: %s", string(status))
	}
	frame.Status = uint8(statusInt)

	payloadLenBytes := make([]byte, 3)
	if _, err = io.ReadFull(reader, payloadLenBytes); err != nil {
		return fmt.Errorf("invalid payload length: %s", string(payloadLenBytes))
	}
	payloadLen, err := strconv.ParseInt(string(payloadLenBytes), 10, 32)
	if err != nil {
		return fmt.Errorf("invalid payload length: %s", string(payloadLenBytes))
	}

	payload := make([]byte, payloadLen)
	if _, err = io.ReadFull(reader, payload); err != nil {
		return err
	}
	frame.Payload = make(map[string]interface{})

	if frame.Function == -1 {
		frame.Payload["error"] = string(payload)
	} else {
		parts := strings.Split(string(payload), ";")
		for _, part := range parts {
			keyValue := strings.SplitN(part, "=", 2)
			if len(keyValue) != 2 {
				continue
			}
			key := strings.ToLower(keyValue[0])
			if frame.Function == 3 {
				values := strings.Split(keyValue[1], ",")
				frame.Payload[key] = make(map[string]interface{})
				frame.Payload[key].(map[string]interface{})["min"] = parseValue(values[0])
				frame.Payload[key].(map[string]interface{})["max"] = parseValue(values[1])
				frame.Payload[key].(map[string]interface{})["default"] = parseValue(values[2])
				frame.Payload[key].(map[string]interface{})["decimals"] = parseValue(values[3])
			} else {
				frame.Payload[key] = parseValue(keyValue[1])
			}
		}
	}

	endMarker := make([]byte, 1)
	if _, err = io.ReadFull(reader, endMarker); err != nil {
		return err
	}
	if endMarker[0] != 0x04 {
		return fmt.Errorf("invalid end marker: %x", endMarker[0])
	}

	return nil
}

func parseValue(value string) interface{} {
	intVal, err := strconv.ParseInt(value, 10, 32)
	if err == nil {
		return intVal
	}
	floatVal, err := strconv.ParseFloat(value, 32)
	if err == nil {
		return RoundedFloat(floatVal)
	}
	return value
}
