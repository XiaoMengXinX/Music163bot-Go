package utils

import (
	"os"
)

func CheckDir(path string) (err error) {
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return
		}
	}
	return
}
