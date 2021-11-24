package util

import (
	"fmt"
	"math"
)

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
