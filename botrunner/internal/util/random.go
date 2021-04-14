package util

import (
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// GetRandomInt returns a random integer in range [min, max].
func GetRandomInt(min int, max int) int {
	return rand.Intn(max-min+1) + min
}

// GetRandomUint32 returns a random integer in range [min, max].
func GetRandomUint32(min uint32, max uint32) uint32 {
	return min + rand.Uint32()%(max-min+1)
}

// GetRandomFloat32 returns a random float32 in range [min, max).
func GetRandomFloat32(min float32, max float32) float32 {
	return min + rand.Float32()*(max-min)
}

// GetRandomMilliseconds returns random time.Duration milliseconds.
func GetRandomMilliseconds(min int, max int) time.Duration {
	ri := GetRandomInt(min, max)
	return time.Duration(ri) * time.Millisecond
}

// Shuffle shuffles the numbers in the slice in place.
func Shuffle(numbers []int) {
	l := len(numbers)
	for i := 0; i < l; i++ {
		randPos := GetRandomInt(0, l-1)
		numbers[i], numbers[randPos] = numbers[randPos], numbers[i]
	}
}
