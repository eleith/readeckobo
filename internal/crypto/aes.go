package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	staticSalt = "88b3a2e13"
)

// deriveKey generates a 16-byte AES key from static salt and Kobo serial.
func deriveKey(koboSerial string) ([]byte, error) {
	data := []byte(staticSalt + koboSerial)
	hash := sha256.Sum256(data)
	hashHex := hex.EncodeToString(hash[:]) // Get full SHA256 hash as hex string

	// Take the first 16 characters of the hex hash string
	intermediateKeyHexStr := hashHex[:16]

	// The actual key bytes are the ASCII values of these 16 characters.
	key := []byte(intermediateKeyHexStr) // This is the 16-byte key

	return key, nil
}

// DecryptAESECB decrypts base64 encoded ciphertext using AES-128-ECB mode with a derived key.
func DecryptAESECB(encryptedTokenB64 string, koboSerial string) (string, error) {
	key, err := deriveKey(koboSerial)
	if err != nil {
		return "", fmt.Errorf("failed to derive key: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedTokenB64)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	blockSize := block.BlockSize()
	if len(ciphertext)%blockSize != 0 {
		return "", fmt.Errorf("ciphertext is not a multiple of the block size")
	}

	// Manual ECB decryption
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += blockSize {
		block.Decrypt(plaintext[i:i+blockSize], ciphertext[i:i+blockSize])
	}

	// Remove PKCS7 padding
	unpaddedPlaintext, err := pkcs7Unpad(plaintext, blockSize)
	if err != nil {
		return "", fmt.Errorf("failed to unpad plaintext: %w", err)
	}

	return string(unpaddedPlaintext), nil
}

// pkcs7Unpad removes PKCS7 padding from decrypted data.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be greater than 0")
	}
	if len(data)%blockSize != 0 || len(data) == 0 {
		return nil, fmt.Errorf("invalid PKCS7 padded data")
	}
	paddingLen := int(data[len(data)-1])
	if paddingLen == 0 || paddingLen > blockSize {
		return nil, fmt.Errorf("invalid PKCS7 padding length")
	}
	// Check all padding bytes are valid
	for i := 0; i < paddingLen; i++ {
		if data[len(data)-1-i] != byte(paddingLen) {
			return nil, fmt.Errorf("invalid PKCS7 padding")
		}
	}
	return data[:len(data)-paddingLen], nil
}

// For testing purposes, matches openssl enc
func encryptAESECB(plaintext string, koboSerial string) (string, error) {
	key, err := deriveKey(koboSerial)
	if err != nil {
		return "", fmt.Errorf("failed to derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	blockSize := block.BlockSize()
	paddedPlaintext := pkcs7Pad([]byte(plaintext), blockSize)

	ciphertext := make([]byte, len(paddedPlaintext))
	for i := 0; i < len(paddedPlaintext); i += blockSize {
		block.Encrypt(ciphertext[i:i+blockSize], paddedPlaintext[i:i+blockSize])
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// pkcs7Pad adds PKCS7 padding to the data.
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

// Temporary main function to print derived keys for testing