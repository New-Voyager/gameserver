package random

import (
	crypto_rand "crypto/rand"
	"math/big"
)

func NewSeed() int64 {
	const MaxUint = ^uint(0)
	const MaxInt = int(MaxUint >> 1)
	nBig, err := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(MaxInt)))
	if err != nil {
		panic("cannot seed math/rand package with cryptographically secure random number generator")
	}

	return nBig.Int64()
}
