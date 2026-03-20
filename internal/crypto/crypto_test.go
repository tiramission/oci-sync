package crypto_test

import (
	"bytes"
	"testing"

	"github.com/tiramission/oci-sync/internal/crypto"
)

func TestEncryptDecrypt(t *testing.T) {
	original := []byte("secret data: hello, oci-sync!")
	passphrase := "my-strong-passphrase"

	encrypted, err := crypto.Encrypt(original, passphrase)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	if bytes.Equal(encrypted, original) {
		t.Fatal("encrypted data should not equal original")
	}

	decrypted, err := crypto.Decrypt(encrypted, passphrase)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}

	if !bytes.Equal(decrypted, original) {
		t.Errorf("decrypted mismatch: got %q, want %q", decrypted, original)
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	data := []byte("some data to encrypt")
	encrypted, err := crypto.Encrypt(data, "correct-pass")
	if err != nil {
		t.Fatal(err)
	}

	_, err = crypto.Decrypt(encrypted, "wrong-pass")
	if err == nil {
		t.Fatal("expected error when decrypting with wrong passphrase")
	}
}

func TestEncryptDifferentEachTime(t *testing.T) {
	data := []byte("same plaintext")
	passphrase := "same-pass"

	enc1, err := crypto.Encrypt(data, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	enc2, err := crypto.Encrypt(data, passphrase)
	if err != nil {
		t.Fatal(err)
	}

	// Due to random salt and nonce, ciphertexts should differ
	if bytes.Equal(enc1, enc2) {
		t.Error("encrypt should produce different output each time (random salt/nonce)")
	}
}

func TestDecryptTooShort(t *testing.T) {
	_, err := crypto.Decrypt([]byte("tooshort"), "pass")
	if err == nil {
		t.Fatal("expected error for too-short ciphertext")
	}
}
