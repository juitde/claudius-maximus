//go:build !windows

package main

import "os"

// installBinary replaces target with data.
//
// A plain rename over target is safe on Unix even while target is currently
// executing: the running process keeps its old inode open under the old
// name, so nothing already running it is disrupted, and every new
// invocation resolves the new file immediately. writeFileAtomic (paths.go)
// already does exactly this - write to a same-directory temp file, then
// rename - so there is nothing platform-specific to add here beyond the
// chmod below.
func installBinary(target string, data []byte, perm os.FileMode) error {
	if err := writeFileAtomic(target, data, perm); err != nil {
		return err
	}
	// writeFileAtomic's temp file has a fixed name (target+".tmp"); if that
	// path already existed from an earlier, interrupted run, a plain
	// os.WriteFile does not change an existing file's permission bits
	// (verified: writing 0o755 over an existing 0o600 file leaves it
	// 0o600). Chmod explicitly so a stale leftover can never produce a
	// binary that silently isn't executable.
	return os.Chmod(target, perm)
}
