package crypto

import (
	"testing"
)

func TestDecryptAESECB(t *testing.T) {
	testCases := []struct {
		name              string
		plaintext         string
		koboSerial        string
		expectedDecrypted string
		expectError       bool
	}{
		{
			name:              "Valid decryption",
			plaintext:         "mysecrettoken",
			koboSerial:        "1234567890abcdef",
			expectedDecrypted: "mysecrettoken",
			expectError:       false,
		},
		{
			name:              "Another valid decryption",
			plaintext:         "another_token_here_123",
			koboSerial:        "fedcba0987654321",
			expectedDecrypted: "another_token_here_123",
			expectError:       false,
		},
		{
			name:              "Empty plaintext",
			plaintext:         "",
			koboSerial:        "emptyserial",
			expectedDecrypted: "",
			expectError:       false,
		},
		{
			name:              "Long plaintext", // Test with a plaintext longer than 16 chars
			plaintext:         "this is a much longer plaintext to test the block cipher and padding",
			koboSerial:        "longserial",
			expectedDecrypted: "this is a much longer plaintext to test the block cipher and padding",
			expectError:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt using the helper function (mimicking openssl)
			encryptedB64, err := encryptAESECB(tc.plaintext, tc.koboSerial)
			if err != nil {
				t.Fatalf("Failed to encrypt for test setup: %v", err)
			}

			// Decrypt using the function under test
			decrypted, err := DecryptAESECB(encryptedB64, tc.koboSerial)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect an error, but got: %v", err)
				}
				if decrypted != tc.expectedDecrypted {
					t.Errorf("Expected decrypted '%s', got '%s'", tc.expectedDecrypted, decrypted)
				}
			}
		})
	}
}

func TestDeriveKey(t *testing.T) {
	testCases := []struct {
		name                 string
		koboSerial           string
		expectedDerivedKey []byte // The 16-byte key as returned by deriveKey
	}{
		{
			name:       "Test serial 1234567890abcdef",
			koboSerial: "1234567890abcdef",
			expectedDerivedKey: []byte("3e489554db28cc3a"),
		},
		{
			name:       "Test serial fedcba0987654321",
			koboSerial: "fedcba0987654321",
			expectedDerivedKey: []byte("fecb7bb07e17ceea"),
		},
		{
			name:       "Test serial short",
			koboSerial: "short",
			expectedDerivedKey: []byte("db886f74f58aae09"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key, err := deriveKey(tc.koboSerial)
			if err != nil {
				t.Fatalf("deriveKey failed: %v", err)
			}
			if len(key) != 16 {
				t.Errorf("Expected key length 16, got %d", len(key))
			}
			if string(key) != string(tc.expectedDerivedKey) {
				t.Errorf("Expected key '%s', got '%s'", string(tc.expectedDerivedKey), string(key))
			}
		})
	}
}
