package util

func ChipsToCents(chips float64) float64 {
	return chips * 100
}

func CentsToChips(cents float64) float64 {
	return cents / 100
}
