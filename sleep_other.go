//go:build !darwin

package main

// checkSystemSleep reports that this platform is not checked, rather than
// omitting the row — silently skipping it would look identical to "checked,
// no problem found", which is not true here. See DEVELOPMENT.md's "Rejected:
// live sleep detection on Linux and Windows" for why: no citable exact output
// format exists to build a real check against, unlike pmset on macOS.
func checkSystemSleep() CheckResult {
	return CheckResult{
		Name: "system sleep", Status: StatusWarn,
		Detail:  "not checked on this platform",
		FixHint: "see the README's 'Preventing sleep' section",
	}
}
