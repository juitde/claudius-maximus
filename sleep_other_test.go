//go:build !darwin

package main

import "testing"

// This runs for real on the ubuntu-latest and windows-latest CI runners, not
// merely as part of the cross-compile job — the "skipped" row is exercised on
// both platforms it actually applies to.
func TestCheckSystemSleep(t *testing.T) {
	check := checkSystemSleep()

	if check.Name != "system sleep" {
		t.Errorf("name = %q", check.Name)
	}
	if check.Status != StatusWarn {
		t.Errorf("status = %v, want warn — silence here would look identical to \"checked, no problem\"", check.Status)
	}
	if check.FixHint == "" {
		t.Error("a skipped check still needs to say where to look instead")
	}
}
