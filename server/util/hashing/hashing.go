package hashing

import (
	"crypto/md5"
	"encoding/hex"
	"hash/fnv"
	"math/rand"

	"voyager.com/logging"
)

var hashingLogger = logging.GetZeroLogger("util::hashing", nil)

func GenerateUint32Hash(data string) uint32 {
	hash := fnv.New32a()
	_, err := hash.Write([]byte(data))
	if err != nil {
		hashingLogger.Warn().Msgf("Could not generate a uint32 hash from data (%s). Using a random number instead.", data)
		return rand.Uint32()
	}
	return hash.Sum32()
}

func GenerateStringHash(data string) string {
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}
