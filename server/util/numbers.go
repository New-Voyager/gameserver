package util

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"

	"voyager.com/logging"
)

var numbersLogger = logging.GetZeroLogger("util::numbers", nil)

const epsilon = 0.000001

func FloorDecimal(num float64, digits int) float64 {
	switch digits {
	case 0:
		return math.Floor(num)
	case 2:
		return math.Floor(num*100) / 100
	default:
		panic(fmt.Sprintf("FloorDecimal digits not supported: %d", digits))
	}
}

func RoundDecimal(num float64, digits int) float64 {
	switch digits {
	case 0:
		return math.Round(num)
	case 2:
		return math.Round(num*100) / 100
	default:
		panic(fmt.Sprintf("RoundDecimal digits not supported: %d", digits))
	}
}

func FloorToNearest(num float64, nearest int) float64 {
	if nearest == 1 {
		return math.Floor(num)
	}
	num = float64(int(num))
	return num - float64(int(num)%nearest)
}

func NearlyEqual(a float64, b float64) bool {
	if a == b {
		return true
	}

	diff := math.Abs(a - b)
	if diff < epsilon {
		return true
	}

	return false
}

func Greater(a float64, b float64) bool {
	return a > b && !NearlyEqual(a, b)
}

func GreaterOrNearlyEqual(a float64, b float64) bool {
	if a > b || a == b {
		return true
	}

	return NearlyEqual(a, b)
}

func SplitCents(totalAmt float64, numSplits int, splits []float64) {
	amountPerSplit := int(totalAmt / float64(numSplits))
	remaining := totalAmt - float64(amountPerSplit*numSplits)
	for i := 0; i < numSplits; i++ {
		splits[i] = float64(amountPerSplit)
		if remaining > 0 {
			splits[i]++
			remaining--
		}
	}
}

func SplitDollars(totalAmt float64, numSplits int, splits []float64) {
	rounded := RoundDecimal(totalAmt, 0)
	if rounded != totalAmt {
		if Env.IsSystemTest() {
			panic(fmt.Sprintf("SplitDollars totalAmt is imprecise. totalAmt: %v", totalAmt))
		} else {
			totalAmt = rounded
		}
	}
	if int(totalAmt)%100 != 0 {
		if Env.IsSystemTest() {
			panic(fmt.Sprintf("SplitDollars totalAmt is not divisible by 100. totalAmt: %v, int(totalAmt): %v", totalAmt, int(totalAmt)))
		}
	}
	amountPerSplit := int(totalAmt / float64(numSplits))
	remainder := amountPerSplit % 100
	if remainder != 0 {
		amountPerSplit = amountPerSplit - remainder
	}
	remaining := totalAmt - float64(amountPerSplit*numSplits)
	for i := 0; i < numSplits; i++ {
		splits[i] = float64(amountPerSplit)
		if remaining > 0 {
			splits[i] += 100
			remaining -= 100
		}
	}
}

func GenerateUint32Hash(data string) uint32 {
	hash := fnv.New32()
	_, err := hash.Write([]byte(data))
	if err != nil {
		numbersLogger.Warn().Msgf("Could not generate a uint32 hash from data (%s). Using a random number instead.", data)
		return rand.Uint32()
	}
	return hash.Sum32()
}
