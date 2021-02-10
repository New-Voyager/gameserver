package util

// ContainsUint32 checks if the slice contains the uint32 value.
func ContainsUint32(slc []uint32, item uint32) bool {
	for _, v := range slc {
		if v == item {
			return true
		}
	}
	return false
}

// ContainsString checks if the slice contains the string value.
func ContainsString(slc []string, item string) bool {
	for _, v := range slc {
		if v == item {
			return true
		}
	}
	return false
}
