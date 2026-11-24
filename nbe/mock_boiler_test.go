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
	"testing"
)

func TestMockBoilerCreation(t *testing.T) {
	mb, err := NewMockBoiler("TEST12345")
	if err != nil {
		t.Fatalf("Failed to create mock boiler: %v", err)
	}

	if mb.Serial != "TEST12345" {
		t.Errorf("Expected serial 'TEST12345', got '%s'", mb.Serial)
	}

	if mb.rsaKeyBase64 == "" {
		t.Error("Expected RSA key to be generated")
	}
}

func TestMockBoilerStartStop(t *testing.T) {
	mb, err := NewMockBoiler("TEST12345")
	if err != nil {
		t.Fatalf("Failed to create mock boiler: %v", err)
	}

	err = mb.Start()
	if err != nil {
		t.Fatalf("Failed to start mock boiler: %v", err)
	}

	if mb.Port == 0 {
		t.Error("Expected port to be assigned")
	}

	if !mb.running {
		t.Error("Expected mock boiler to be running")
	}

	mb.Stop()

	if mb.running {
		t.Error("Expected mock boiler to be stopped")
	}
}

func TestMockBoilerDiscovery(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

func TestMockBoilerGetOperatingData(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

func TestMockBoilerGetSetupData(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

func TestMockBoilerSetValue(t *testing.T) {
	mb, err := NewMockBoiler("TEST12345")
	if err != nil {
		t.Fatalf("Failed to create mock boiler: %v", err)
	}

	testValue := RoundedFloat(99.9)
	mb.SetValue("boiler", "temp", testValue)

	retrievedValue, ok := mb.GetValue("boiler", "temp")
	if !ok {
		t.Fatal("Failed to retrieve set value")
	}

	if retrievedValue != testValue {
		t.Errorf("Expected value %v, got %v", testValue, retrievedValue)
	}
}

func TestMockBoilerAsyncRequests(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}

func TestMockBoilerMultipleClients(t *testing.T) {
	t.Skip("Skipping integration test - requires working network communication")
}
