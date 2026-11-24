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
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Frame field sizes for NBE protocol
const (
	AppIDSize        = 12
	ControllerIDSize = 6
	FunctionSize     = 2
	SeqNoSize        = 2
	PinCodeSize      = 10
	TimestampSize    = 10
	ExtrMarkerSize   = 4
	PayloadLenSize   = 3
	StatusSize       = 1
)

// Protocol markers
const (
	StartMarker byte = 0x02
	EndMarker   byte = 0x04
)

// Binary read helpers using binary.Read (for requests)

func readBytes(r io.Reader, n int, fieldName string) ([]byte, error) {
	buf := make([]byte, n)
	if err := binary.Read(r, binary.BigEndian, &buf); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fieldName, err)
	}
	return buf, nil
}

func readString(r io.Reader, n int, fieldName string) (string, error) {
	buf, err := readBytes(r, n, fieldName)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// Full read helpers using io.ReadFull (for responses)

func readBytesFull(r io.Reader, n int, fieldName string) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fieldName, err)
	}
	return buf, nil
}

func readStringFull(r io.Reader, n int, fieldName string) (string, error) {
	buf, err := readBytesFull(r, n, fieldName)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// ASCII integer reading helpers

func readASCIIInt(r io.Reader, n int, fieldName string) (int, error) {
	buf, err := readBytes(r, n, fieldName)
	if err != nil {
		return 0, err
	}
	var value int
	if _, err := fmt.Sscanf(string(buf), "%d", &value); err != nil {
		return 0, fmt.Errorf("failed to parse %s as integer: %w", fieldName, err)
	}
	return value, nil
}

func readASCIIInt8(r io.Reader, n int, fieldName string) (int8, error) {
	value, err := readASCIIInt(r, n, fieldName)
	return int8(value), err
}

func readASCIIInt16(r io.Reader, n int, fieldName string) (Function, error) {
	value, err := readASCIIInt(r, n, fieldName)
	return Function(value), err
}

func readASCIIInt64(r io.Reader, n int, fieldName string) (int64, error) {
	buf, err := readBytes(r, n, fieldName)
	if err != nil {
		return 0, err
	}
	var value int64
	if _, err := fmt.Sscanf(string(buf), "%d", &value); err != nil {
		return 0, fmt.Errorf("failed to parse %s as int64: %w", fieldName, err)
	}
	return value, nil
}

func readASCIIInt64Full(r io.Reader, n int, fieldName string) (int64, error) {
	buf, err := readBytesFull(r, n, fieldName)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(buf)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s as integer: %w", fieldName, err)
	}
	return value, nil
}

// Marker validation helpers

func readAndValidateMarker(r io.Reader, expected byte, fieldName string) error {
	// Skip encryption byte first
	if _, err := readBytes(r, 1, "encryption"); err != nil {
		return err
	}
	// Read and validate marker
	marker, err := readBytes(r, 1, fieldName)
	if err != nil {
		return err
	}
	if marker[0] != expected {
		return fmt.Errorf("invalid %s: expected 0x%02x, got 0x%02x", fieldName, expected, marker[0])
	}
	return nil
}

func validateMarker(r io.Reader, expected byte, fieldName string) error {
	marker, err := readBytesFull(r, 1, fieldName)
	if err != nil {
		return err
	}
	if marker[0] != expected {
		return fmt.Errorf("invalid %s: expected 0x%02x, got 0x%02x", fieldName, expected, marker[0])
	}
	return nil
}

// Binary write helpers

func writeString(w io.Writer, s string, size int, fieldName string) error {
	formatted := fmt.Sprintf("%*s", size, s)
	if len(formatted) > size {
		formatted = formatted[:size]
	}
	if err := binary.Write(w, binary.BigEndian, []byte(formatted)); err != nil {
		return fmt.Errorf("failed to write %s: %w", fieldName, err)
	}
	return nil
}

func writeByte(w io.Writer, b byte, fieldName string) error {
	if err := binary.Write(w, binary.BigEndian, [1]byte{b}); err != nil {
		return fmt.Errorf("failed to write %s: %w", fieldName, err)
	}
	return nil
}

func writeASCIIInt(w io.Writer, value, size int, fieldName string) error {
	formatted := fmt.Sprintf("%0*d", size, value)
	if len(formatted) > size {
		return fmt.Errorf("%s value %d too large for %d digits", fieldName, value, size)
	}
	if err := binary.Write(w, binary.BigEndian, []byte(formatted)); err != nil {
		return fmt.Errorf("failed to write %s: %w", fieldName, err)
	}
	return nil
}

func writeRawBytes(w io.Writer, data []byte, fieldName string) error {
	if err := binary.Write(w, binary.BigEndian, data); err != nil {
		return fmt.Errorf("failed to write %s: %w", fieldName, err)
	}
	return nil
}

// Payload serialization helper

func serializePayload(payload map[string]interface{}) string {
	if len(payload) == 0 {
		return ""
	}
	var parts []string
	for key, value := range payload {
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}
	return strings.Join(parts, ";")
}

// Payload parsing helpers

func parsePayload(payloadStr string, function Function) map[string]interface{} {
	payload := make(map[string]interface{})

	if function == UnknownFunction {
		payload["error"] = payloadStr
		return payload
	}

	parts := strings.Split(payloadStr, ";")
	for _, part := range parts {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) != 2 {
			continue
		}
		key := strings.ToLower(keyValue[0])
		if function == GetSetupRangeFunction {
			values := strings.Split(keyValue[1], ",")
			if len(values) >= 4 {
				payload[key] = map[string]interface{}{
					"min":      parseValue(values[0]),
					"max":      parseValue(values[1]),
					"default":  parseValue(values[2]),
					"decimals": parseValue(values[3]),
				}
			}
		} else {
			payload[key] = parseValue(keyValue[1])
		}
	}

	return payload
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
