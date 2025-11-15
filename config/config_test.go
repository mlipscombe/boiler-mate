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

package config

import (
	"os"
	"testing"
)

func TestLookupEnvOrString(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultVal   string
		expected     string
		shouldSetEnv bool
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_KEY_NOT_SET",
			defaultVal:   "default_value",
			expected:     "default_value",
			shouldSetEnv: false,
		},
		{
			name:         "returns env value when set",
			key:          "TEST_KEY_SET",
			envValue:     "env_value",
			defaultVal:   "default_value",
			expected:     "env_value",
			shouldSetEnv: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSetEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := lookupEnvOrString(tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("lookupEnvOrString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLookupEnvOrBool(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultVal   bool
		expected     bool
		shouldSetEnv bool
	}{
		{
			name:         "returns default when env not set",
			key:          "TEST_BOOL_NOT_SET",
			defaultVal:   true,
			expected:     true,
			shouldSetEnv: false,
		},
		{
			name:         "returns true for 'true'",
			key:          "TEST_BOOL_TRUE",
			envValue:     "true",
			defaultVal:   false,
			expected:     true,
			shouldSetEnv: true,
		},
		{
			name:         "returns true for '1'",
			key:          "TEST_BOOL_ONE",
			envValue:     "1",
			defaultVal:   false,
			expected:     true,
			shouldSetEnv: true,
		},
		{
			name:         "returns true for 'yes'",
			key:          "TEST_BOOL_YES",
			envValue:     "yes",
			defaultVal:   false,
			expected:     true,
			shouldSetEnv: true,
		},
		{
			name:         "returns false for 'false'",
			key:          "TEST_BOOL_FALSE",
			envValue:     "false",
			defaultVal:   true,
			expected:     false,
			shouldSetEnv: true,
		},
		{
			name:         "returns false for any other value",
			key:          "TEST_BOOL_OTHER",
			envValue:     "whatever",
			defaultVal:   true,
			expected:     false,
			shouldSetEnv: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldSetEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := lookupEnvOrBool(tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("lookupEnvOrBool() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Note: Tests that call Load() can only run once per test binary
// due to flag.Parse() being called which cannot be reset.
// These tests should be run separately or as integration tests.

func TestConfigDefaults(t *testing.T) {
	t.Skip("Skipping due to flag redefinition - run as integration test")
}

func TestConfigEnvironmentOverride(t *testing.T) {
	t.Skip("Skipping due to flag redefinition - run as integration test")
}
