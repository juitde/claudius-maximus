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
// by name. The swap is therefore: move the running exe aside, then place
// the new content at target the same way writeFileAtomic (paths.go) always
// does - a same-directory temp file, then a rename, which now succeeds
// because target no longer exists. If that fails, the aside file is
// renamed back so a failed update never leaves target missing, and that
// rollback's own success or failure is always part of the reported error -
// a silent rollback failure would otherwise leave the binary gone with no
// indication why.
//
// The aside name is qualified with this process's PID, and any aside file
// left behind by an earlier run is swept (best-effort) before a new one is
// created - both bound how many can ever accumulate, since deleting a
// still-running exe reliably fails and a leftover otherwise has no other
// path back to being cleaned up.
func installBinary(target string, data []byte, perm os.FileMode) error {
	removeStaleAsideFiles(target)

	aside := fmt.Sprintf("%s.old.%d", target, os.Getpid())
	if err := os.Rename(target, aside); err != nil {
		return fmt.Errorf("move %s aside before replacing it: %w", target, err)
	}

	if err := writeFileAtomic(target, data, perm); err != nil {
		if rollbackErr := os.Rename(aside, target); rollbackErr != nil {
			return fmt.Errorf(
				"replace %s failed (%w), and restoring the previous binary also failed (%v) - it is still at %s",
				target, err, rollbackErr, aside)
		}
		return fmt.Errorf("replace %s: %w", target, err)
	}

	_ = os.Remove(aside) // best-effort: a still-running old process may hold this open
	return nil
}

// removeStaleAsideFiles best-effort-removes .old.<pid> files installBinary
// left behind on earlier runs. A file still legitimately in use by a
// process that has not exited yet is expected to fail here and is simply
// retried on some future call.
func removeStaleAsideFiles(target string) {
	matches, err := filepath.Glob(target + ".old.*")
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}
