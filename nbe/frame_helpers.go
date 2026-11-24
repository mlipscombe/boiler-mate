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
	"math"
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

func readBytes(r io.Reader, n int, fieldName string) ([]byte, error) {
	if n < 0 || n > 999 {
		return nil, fmt.Errorf("invalid %s length", fieldName)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read %s: %w", fieldName, err)
	}
	return buf, nil
}

func readString(r io.Reader, n int, fieldName string) (string, error) {
	buf, err := readBytes(r, n, fieldName)
	return string(buf), err
}

func readASCIIInt(r io.Reader, n int, fieldName string) (int, error) {
	value, err := readASCIIInt64(r, n, fieldName)
	return int(value), err
}

func readASCIIInt8(r io.Reader, n int, fieldName string) (int8, error) {
	value, err := readASCIIInt64(r, n, fieldName)
	return int8(value), err
}

func readASCIIInt16(r io.Reader, n int, fieldName string) (Function, error) {
	value, err := readASCIIInt64(r, n, fieldName)
	return Function(value), err
}

func readASCIIInt64(r io.Reader, n int, fieldName string) (int64, error) {
	field, err := readString(r, n, fieldName)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(strings.TrimSpace(field), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s field", fieldName)
	}
	return value, nil
}

func validateMarker(r io.Reader, expected byte, fieldName string) error {
	marker, err := readBytes(r, 1, fieldName)
	if err != nil {
		return err
	}
	if marker[0] != expected {
		return fmt.Errorf("invalid %s: expected 0x%02x, got 0x%02x", fieldName, expected, marker[0])
	}
	return nil
}

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
	intVal, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return intVal
	}
	floatVal, err := strconv.ParseFloat(value, 32)
	if err == nil && !math.IsNaN(floatVal) && !math.IsInf(floatVal, 0) {
		return RoundedFloat(floatVal)
	}
	return value
}
