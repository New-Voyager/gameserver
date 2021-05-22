package util

import (
	"fmt"

	"gopkg.in/godo.v2/glob"
)

// GetFilesInDir returns file names in the directory. It does not look into the subdirectories.
func GetFilesInDir(dirName string) ([]string, error) {
	var files []string
	pattern := fmt.Sprintf("%s/**/*.yaml", dirName)
	patterns := []string{pattern}
	globFiles, _, err := glob.Glob(patterns)
	if err != nil {
		return nil, fmt.Errorf("Failed to get files from dir: %s", dirName)
	}
	for _, file := range globFiles {
		if file.IsDir() {
			continue
		}
		files = append(files, file.Path)
	}
	return files, nil
}
