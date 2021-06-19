package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"io"
)

func EncryptWithPlayerID(data []byte, playerID uint64) ([]byte, error) {
	key := make([]byte, 32)

	// For exmaple player ID 31, key is
	// [31 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0]
	binary.LittleEndian.PutUint64(key, playerID)
	return Encrypt(data, key)
}

func DecryptWithPlayerID(data []byte, playerID uint64) ([]byte, error) {
	key := make([]byte, 32)
	binary.LittleEndian.PutUint64(key, playerID)
	return Decrypt(data, key)
}

// Encrypt encrypts the data. Key must be 32 bytes.
func Encrypt(data []byte, key []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	encrypted := gcm.Seal(nonce, nonce, data, nil)
	return encrypted, nil
}

// Decrypt decrypts the data. Key must be 32 bytes.
func Decrypt(data []byte, key []byte) ([]byte, error) {
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, err
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return decrypted, nil
}
