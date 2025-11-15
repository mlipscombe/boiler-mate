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
	"fmt"
	"io"
)

// Use shared constants from frame_helpers.go

type NBEResponse struct {
	AppID        string // client application id
	ControllerID string // controller id
	Function     Function
	SeqNo        int8
	Status       uint8
	Payload      map[string]interface{}
}

func (frame *NBEResponse) Pack(writer io.Writer) error {
	// Write header fields
	if err := writeString(writer, frame.AppID, AppIDSize, "AppID"); err != nil {
		return err
	}
	if err := writeString(writer, frame.ControllerID, ControllerIDSize, "ControllerID"); err != nil {
		return err
	}

	// Write start marker
	if err := writeByte(writer, StartMarker, "start marker"); err != nil {
		return err
	}

	// Write function and sequence number (ASCII encoded)
	if err := writeASCIIInt(writer, int(frame.Function), FunctionSize, "Function"); err != nil {
		return err
	}
	if err := writeASCIIInt(writer, int(frame.SeqNo), SeqNoSize, "SeqNo"); err != nil {
		return err
	}

	// Write status
	if err := writeASCIIInt(writer, int(frame.Status), StatusSize, "Status"); err != nil {
		return err
	}

	// Serialize and write payload
	payloadStr := serializePayload(frame.Payload)
	if err := writeASCIIInt(writer, len(payloadStr), PayloadLenSize, "payload length"); err != nil {
		return err
	}
	if err := writeRawBytes(writer, []byte(payloadStr), "payload"); err != nil {
		return err
	}

	// Write end marker
	if err := writeByte(writer, EndMarker, "end marker"); err != nil {
		return err
	}

	return nil
}

func (frame *NBEResponse) Unpack(reader io.Reader) error {
	// Read header fields
	var err error
	if frame.AppID, err = readStringFull(reader, AppIDSize, "AppID"); err != nil {
		return err
	}
	if frame.ControllerID, err = readStringFull(reader, ControllerIDSize, "ControllerID"); err != nil {
		return err
	}

	// Validate start marker
	if err := validateMarker(reader, StartMarker, "start marker"); err != nil {
		return err
	}

	// Read function, sequence number, and status (ASCII encoded)
	var functionInt int64
	if functionInt, err = readASCIIInt64Full(reader, FunctionSize, "Function"); err != nil {
		frame.Function = UnknownFunction
	} else {
		frame.Function = Function(functionInt)
	}

	var seqNoInt int64
	if seqNoInt, err = readASCIIInt64Full(reader, SeqNoSize, "SeqNo"); err != nil {
		frame.SeqNo = -1
	} else {
		frame.SeqNo = int8(seqNoInt)
	}

	var statusInt int64
	if statusInt, err = readASCIIInt64Full(reader, StatusSize, "Status"); err != nil {
		return fmt.Errorf("invalid status: %w", err)
	}
	frame.Status = uint8(statusInt)

	// Read payload length and payload
	var payloadLen int64
	if payloadLen, err = readASCIIInt64Full(reader, PayloadLenSize, "payload length"); err != nil {
		return fmt.Errorf("invalid payload length: %w", err)
	}

	payloadBytes, err := readBytesFull(reader, int(payloadLen), "payload")
	if err != nil {
		return err
	}

	// Parse payload
	frame.Payload = parsePayload(string(payloadBytes), frame.Function)

	// Validate end marker
	if err := validateMarker(reader, EndMarker, "end marker"); err != nil {
		return err
	}

	return nil
}

// Helper functions are now in frame_helpers.go
