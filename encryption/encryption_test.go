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

func TestEncryptDecryptWithPlayerID(t *testing.T) {
	originalText := []byte("Encrypt text using the AES - Advanced Encryption Standard in Go")
	var player1ID uint64 = 1
	var player2ID uint64 = 2

	encrypted1, err := EncryptWithPlayerID(originalText, player1ID)
	if err != nil {
		print(err)
		t.Error(err)
	}

	encrypted2, err := EncryptWithPlayerID(originalText, player2ID)
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

	decrypted1, err := DecryptWithPlayerID(encrypted1, player1ID)
	if err != nil {
		print(err)
		t.Error(err)
	}

	if !cmp.Equal(decrypted1, originalText) {
		t.Errorf("%s != %s", decrypted1, originalText)
	}

	// Decrypt with wrong player ID. Should error.
	_, err = DecryptWithPlayerID(encrypted1, player2ID)
	if err == nil {
		print(err)
		t.Error(err)
	}
}
