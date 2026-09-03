//go:build windows

package safeio

import "os"

func openRegularNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
