package safeio

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type FileEntry struct {
	Relative string
	Size     int64
	ModTime  time.Time
}

func ListRegularFilesWithin(root, relative string, maxFiles int) ([]FileEntry, error) {
	if filepath.IsAbs(relative) || maxFiles < 1 {
		return nil, errors.New("invalid file listing request")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil || !rootInfo.IsDir() {
		return nil, errors.New("provider root is not a directory")
	}
	start := filepath.Join(resolvedRoot, relative)
	startInfo, err := os.Lstat(start)
	if errors.Is(err, os.ErrNotExist) {
		return []FileEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	if startInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("symlink is not allowed in provider data")
	}
	resolvedStart, err := filepath.EvalSymlinks(start)
	if err != nil || !pathWithin(resolvedRoot, resolvedStart) {
		return nil, errors.New("listing path escapes provider root")
	}
	entries := []FileEntry{}
	err = filepath.WalkDir(resolvedStart, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink is not allowed in provider data")
		}
		if entry.IsDir() {
			resolved, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil || !pathWithin(resolvedRoot, resolved) {
				return errors.New("directory escapes provider root")
			}
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return errors.New("special file is not allowed in provider data")
		}
		if len(entries) >= maxFiles {
			return errors.New("file count exceeds limit")
		}
		rel, relErr := filepath.Rel(resolvedRoot, path)
		if relErr != nil || !pathWithin(resolvedRoot, path) {
			return errors.New("file escapes provider root")
		}
		entries = append(entries, FileEntry{Relative: rel, Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	postRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	postInfo, err := os.Stat(postRoot)
	if err != nil || !os.SameFile(rootInfo, postInfo) {
		return nil, errors.New("provider root changed while listing")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Relative < entries[j].Relative })
	return entries, nil
}
