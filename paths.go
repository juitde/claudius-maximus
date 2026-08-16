package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// envHome overrides the state directory. Useful for testing and for anyone who
// keeps dotfiles somewhere other than the home directory.
const envHome = envPrefix + "HOME"

// resolveStateDir returns the directory holding config.json, projects.json,
// sessions.json and the session logs.
func resolveStateDir() (string, error) {
	if dir := os.Getenv(envHome); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory (set %s to override): %w", envHome, err)
	}
	return filepath.Join(home, "."+appName), nil
}

// writeFileAtomic writes data to path via a temporary file in the same
// directory followed by a rename, so a reader never observes a half-written
// file and a crash mid-write cannot destroy the previous contents.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Leaving the temporary file behind would make the next write fail in
		// confusing ways, so clean up before reporting.
		os.Remove(tmp)
		return err
	}
	return nil
}
