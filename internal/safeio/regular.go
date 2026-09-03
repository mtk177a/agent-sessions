package safeio

import (
	"errors"
	"os"
	"path/filepath"
)

type fileOpener func(string) (*os.File, error)

func openVerifiedRegular(path string, opener fileOpener) (*os.File, string, error) {
	preInfo, err := os.Lstat(path)
	if err != nil {
		return nil, "", err
	}
	if !preInfo.Mode().IsRegular() {
		return nil, "", errors.New("input is not a regular file")
	}
	file, err := opener(path)
	if err != nil {
		return nil, "", err
	}
	closeOnError := func(err error) (*os.File, string, error) {
		_ = file.Close()
		return nil, "", err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		return closeOnError(err)
	}
	if !openedInfo.Mode().IsRegular() {
		return closeOnError(errors.New("input is not a regular file"))
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return closeOnError(err)
	}
	postInfo, err := os.Stat(resolved)
	if err != nil {
		return closeOnError(err)
	}
	if !os.SameFile(openedInfo, postInfo) {
		return closeOnError(errors.New("input changed while opening"))
	}
	return file, resolved, nil
}

func readRegularFile(path string, maxBytes int64) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	file, _, err := openVerifiedRegular(resolved, openRegularNoFollow)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ioReadAllLimit(file, maxBytes)
}
