package util

func ChipsToCents(chips float64) float64 {
	return RoundDecimal(chips*100, 0)
}

func CentsToChips(cents float64) float64 {
	return cents / 100
}
