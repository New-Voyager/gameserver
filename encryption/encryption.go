package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

func EncryptWithUUIDStrKey(data []byte, uuidStr string) ([]byte, error) {
	var key uuid.UUID = uuid.MustParse(uuidStr)
	return EncryptWithUUIDKey(data, key)
}

func DecryptWithUUIDStrKey(data []byte, uuidStr string) ([]byte, error) {
	var key uuid.UUID = uuid.MustParse(uuidStr)
	return DecryptWithUUIDKey(data, key)
}

func EncryptWithUUIDKey(data []byte, _uuid uuid.UUID) ([]byte, error) {
	key, err := uuidToBytes(_uuid)
	if err != nil {
		return nil, err
	}
	return Encrypt(data, key)
}

func DecryptWithUUIDKey(data []byte, _uuid uuid.UUID) ([]byte, error) {
	key, err := uuidToBytes(_uuid)
	if err != nil {
		return nil, err
	}
	return Decrypt(data, key)
}

func uuidToBytes(_uuid uuid.UUID) ([]byte, error) {
	bytes, err := _uuid.MarshalBinary()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to convert encryption key (uuid) to bytes")
	}

	return bytes, nil
}

// Encrypt encrypts the data.
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
