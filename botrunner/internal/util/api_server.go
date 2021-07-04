package util

import (
	"fmt"
	"path"
	"strings"
)

// GetGqlURL returns the graphql URL.
func GetGqlURL(apiServerURL string) string {
	return joinURL(apiServerURL, "/graphql")
}

func GetInternalRestURL(apiServerURL string) string {
	return joinURL(apiServerURL, "/bot-script")
}

func joinURL(base string, paths ...string) string {
	p := path.Join(paths...)
	return fmt.Sprintf("%s/%s", strings.TrimRight(base, "/"), strings.TrimLeft(p, "/"))
}
