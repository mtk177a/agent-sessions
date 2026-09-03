package safeio

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

func ReadFileWithin(root, relative string, maxBytes int64) ([]byte, error) {
	return readFileWithin(root, relative, maxBytes, openRegularNoFollow)
}

func readFileWithin(root, relative string, maxBytes int64, opener fileOpener) ([]byte, error) {
	if filepath.IsAbs(relative) {
		return nil, errors.New("relative path must not be absolute")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() {
		return nil, errors.New("provider root is not a directory")
	}
	rootFile, err := os.Open(resolvedRoot)
	if err != nil {
		return nil, err
	}
	defer rootFile.Close()
	openedRootInfo, err := rootFile.Stat()
	if err != nil || !os.SameFile(rootInfo, openedRootInfo) {
		return nil, errors.New("provider root changed while opening")
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, relative))
	if err != nil {
		return nil, err
	}
	if !pathWithin(resolvedRoot, candidate) {
		return nil, errors.New("path escapes provider root")
	}
	file, openedPath, err := openVerifiedRegular(candidate, opener)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	postRoot, err := filepath.EvalSymlinks(resolvedRoot)
	if err != nil {
		return nil, err
	}
	postRootInfo, err := os.Stat(postRoot)
	if err != nil || !os.SameFile(openedRootInfo, postRootInfo) {
		return nil, errors.New("provider root changed while opening")
	}
	if !pathWithin(postRoot, openedPath) {
		return nil, errors.New("path escapes provider root")
	}
	data, err := ioReadAllLimit(file, maxBytes)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !(len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator))
}

func ioReadAllLimit(file *os.File, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("input exceeds size limit")
	}
	return data, nil
}
