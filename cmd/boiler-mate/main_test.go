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

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIHelpAndRedaction(t *testing.T) {
	var stderr bytes.Buffer
	if got := run(t.Context(), []string{"--help"}, &stderr); got != 0 || !strings.Contains(stderr.String(), "web-bind") {
		t.Fatalf("expected successful help, got %d: %s", got, &stderr)
	}
	stderr.Reset()
	if got := run(t.Context(), []string{"--mqtt", "mqtt://user:PRIVATE@%zz"}, &stderr); got != 2 || strings.Contains(stderr.String(), "PRIVATE") {
		t.Fatalf("expected sanitized CLI error, got %d: %s", got, &stderr)
	}
}
