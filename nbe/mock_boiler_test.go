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
	"net/url"
	"testing"
)

func TestMockBoilerReadOnlyRoundTrip(t *testing.T) {
	mock, err := NewMockBoiler("SIM123")
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Stop)
	client, err := Open(t.Context(), &url.URL{Scheme: "udp", Host: mock.GetAddr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if client.Serial != "SIM123" {
		t.Errorf("serial = %q", client.Serial)
	}
	reply, err := client.Get(t.Context(), GetOperatingDataFunction, "*")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reply.Payload["boiler_temp"]; !ok {
		t.Error("boiler temperature was missing")
	}
	if mock.WriteAttempts() != 0 {
		t.Fatal("the controller received a settings write")
	}
}
