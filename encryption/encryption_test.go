package encryption

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEncryptDecrypt(t *testing.T) {
	originalText := []byte("Encrypt text using the AES - Advanced Encryption Standard in Go")
	key := []byte("passphrasewhichneedstobe32bytes!")

	encrypted, err := Encrypt(originalText, key)
	if err != nil {
		print(err)
		t.Error(err)
	}

	if cmp.Equal(encrypted, originalText) {
		t.Errorf("%s == %s", encrypted, originalText)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		print(err)
		t.Error(err)
	}

	if !cmp.Equal(decrypted, originalText) {
		t.Errorf("%s != %s", decrypted, originalText)
	}
}

func TestEncryptDecryptWithUUIDStrKey(t *testing.T) {
	originalText := []byte("Encrypt text using the AES - Advanced Encryption Standard in Go")
	key1 := "7faadaf6-ed32-47a9-a09a-01fd0daf9c3f"
	key2 := "b42ac4a3-8789-4f6e-98ca-2e829478e362"

	encrypted1, err := EncryptWithUUIDStrKey(originalText, key1)
	if err != nil {
		print(err)
		t.Error(err)
	}

	encrypted2, err := EncryptWithUUIDStrKey(originalText, key2)
	if err != nil {
		print(err)
		t.Error(err)
	}

	if cmp.Equal(encrypted1, originalText) {
		t.Errorf("%s == %s", encrypted1, originalText)
	}

	if cmp.Equal(encrypted1, encrypted2) {
		t.Errorf("%s == %s", encrypted1, encrypted2)
	}

	decrypted1, err := DecryptWithUUIDStrKey(encrypted1, key1)
	if err != nil {
		print(err)
		t.Error(err)
	}

	if !cmp.Equal(decrypted1, originalText) {
		t.Errorf("%s != %s", decrypted1, originalText)
	}

	// Decrypt with wrong player ID. Should error.
	_, err = DecryptWithUUIDStrKey(encrypted1, key2)
	if err == nil {
		print(err)
		t.Error(err)
	}
}
