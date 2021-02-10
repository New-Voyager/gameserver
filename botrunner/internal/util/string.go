package util

import (
	"regexp"
)

// IsAlphaNumericUnderscore checks if the string contains only alphabet, numbers, and _.
func IsAlphaNumericUnderscore(s string) bool {
	var matcher = regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString
	return matcher(s)
}
