package util

import (
	"io/ioutil"
)

// GetFilesInDir returns file names in the directory. It does not look into the subdirectories.
func GetFilesInDir(dirName string) ([]string, error) {
	var files []string
	fileInfos, err := ioutil.ReadDir(dirName)
	if err != nil {
		return nil, err
	}
	for _, info := range fileInfos {
		if info.IsDir() {
			continue
		}
		files = append(files, info.Name())
	}
	return files, nil
}
