//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// installBinary replaces target with data.
//
// Unlike Unix, Windows will not let a plain rename overwrite a file that is
// currently open for execution - but it will let that file be renamed aside
// under a different name, since Windows tracks an open file by handle, not
// by name. The swap is therefore: move the running exe aside, write the new
// content into a same-directory temp file, then rename that into place at
// target. If the second rename fails, the aside file is renamed back so a
// failed update never leaves target missing.
//
// The aside name is qualified with this process's PID so it can never
// collide with one left over from a previous run - which would otherwise
// make the very first rename fail too, since Windows errors renaming onto
// an existing destination. Deleting a still-running exe reliably fails, so
// a leftover aside file has to be tolerated; it just must never be given a
// name that blocks a future update.
func installBinary(target string, data []byte, perm os.FileMode) error {
	aside := fmt.Sprintf("%s.old.%d", target, os.Getpid())

	if err := os.Rename(target, aside); err != nil {
		return fmt.Errorf("move %s aside before replacing it: %w", target, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".new-*")
	if err != nil {
		_ = os.Rename(aside, target)
		return err
	}
	tmpPath := tmp.Name()

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = os.Chmod(tmpPath, perm)
	}
	if writeErr != nil {
		os.Remove(tmpPath)
		_ = os.Rename(aside, target)
		return writeErr
	}

	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		if rollbackErr := os.Rename(aside, target); rollbackErr != nil {
			return fmt.Errorf(
				"replace %s failed (%w) and restoring the previous binary also failed (%v) - it is still at %s",
				target, err, rollbackErr, aside)
		}
		return fmt.Errorf("replace %s: %w", target, err)
	}

	_ = os.Remove(aside) // best-effort: a still-running old process may hold this open
	return nil
}
