package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// fileIdentity returns a device+inode key, so the same bytes reached through a
// hardlink under a different name resolve to one identity.
func fileIdentity(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("cannot identify %s", path)
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), nil
}

// hasKnownFile reports whether anything under root is already in the catalog.
func hasKnownFile(root string, known map[string]bool) bool {
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil //nolint:nilerr // an unreadable subtree must not abort the walk
		}
		if id, err := fileIdentity(p); err == nil && known[id] {
			found = true
		}
		return nil
	})
	return found
}
