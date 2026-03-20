// Package crypto provides AES-256-GCM encryption and decryption with
// scrypt-based key derivation from a passphrase.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize  = 32
	nonceSize = 12
	keySize   = 32 // AES-256

	// scrypt parameters (N=2^15, r=8, p=1) — OWASP recommended minimum
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

// Encrypt encrypts data using AES-256-GCM.
// The returned bytes are formatted as: [salt(32B)][nonce(12B)][ciphertext+tag].
func Encrypt(data []byte, passphrase string) ([]byte, error) {
	// Generate random salt
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Layout: salt | nonce | ciphertext+tag
	result := make([]byte, 0, saltSize+nonceSize+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

// Decrypt decrypts data produced by Encrypt.
func Decrypt(data []byte, passphrase string) ([]byte, error) {
	minLen := saltSize + nonceSize + 16 // 16 = GCM tag size
	if len(data) < minLen {
		return nil, fmt.Errorf("ciphertext too short")
	}

	salt := data[:saltSize]
	nonce := data[saltSize : saltSize+nonceSize]
	ciphertext := data[saltSize+nonceSize:]

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w (wrong passphrase?)", err)
	}
	return plaintext, nil
}

// deriveKey derives a 32-byte AES key from a passphrase and salt using scrypt.
func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, keySize)
	if err != nil {
		return nil, fmt.Errorf("scrypt key derivation: %w", err)
	}
	return key, nil
}
