package util

func ChipsToCents(chipUnit int32, chips float64) float64 {
	if chipUnit == 0 {
		// Dollar
		return chips * 100
	}
	return chips
}

func CentsToChips(chipUnit int32, cents float64) float64 {
	if chipUnit == 0 {
		// Dollar
		return cents / 100
	}
	return cents
}
