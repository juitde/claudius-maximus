//go:build !darwin

package main

// checkSystemSleep has nothing to check outside darwin — see sleep_darwin.go.
// The bool return means Doctor simply adds no row here, rather than one that
// says nothing useful.
func checkSystemSleep() (CheckResult, bool) {
	return CheckResult{}, false
}
