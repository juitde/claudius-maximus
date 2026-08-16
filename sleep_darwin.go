//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
)

// checkSystemSleep reports whether the system is set up so it will not sleep,
// which matters because a sleeping machine suspends every running
// environment along with it — an idle-sleeping laptop can end a session for
// no reason its user did anything to cause.
//
// darwin-only: pmset is a macOS tool, and Linux/Windows have no single
// equivalent this project could verify against — see sleep_other.go, which
// reports "not checked" there rather than omitting the row.
//
// Advisory only — never StatusFail. Whether the machine is allowed to sleep
// is the user's call; this only makes sure they can see the consequence
// before it surprises them, which is what makes it worth doing at all: for
// remote-control environments specifically, sleep is not a pause, it is the
// process losing its network connection to the phone.
func checkSystemSleep() CheckResult {
	out, err := exec.Command("pmset", "-g").Output()
	if err != nil {
		return CheckResult{
			Name: "system sleep", Status: StatusWarn,
			Detail: "could not run pmset -g: " + err.Error(),
		}
	}

	minutes, note, ok := parseSleepSetting(string(out))
	if !ok {
		return CheckResult{
			Name: "system sleep", Status: StatusWarn,
			Detail: "could not parse pmset -g output",
		}
	}

	if minutes == 0 {
		detail := "disabled"
		if note != "" {
			detail = note
		}
		return CheckResult{Name: "system sleep", Status: StatusOK, Detail: detail}
	}
	return CheckResult{
		Name: "system sleep", Status: StatusWarn,
		Detail: fmt.Sprintf(
			"the system sleeps after %d minutes idle, which suspends every running environment and drops its connection",
			minutes),
		FixHint: "caffeinate -s (while a terminal stays open), or System Settings > Battery > set sleep to Never",
	}
}

// sleepLinePattern matches pmset -g's "sleep" line and nothing else. The key
// has to be anchored at the start of the line: disksleep, displaysleep and
// networkoversleep are separate settings that also contain "sleep" and would
// otherwise match too. pmset resolves an active caffeinate (or any other)
// assertion into this same line — e.g. "sleep  0 (sleep prevented by
// caffeinate, caffeinate)" — so nothing else needs to be consulted separately.
var sleepLinePattern = regexp.MustCompile(`(?m)^\s*sleep\s+(\d+)\s*(?:\(([^)]*)\))?`)

func parseSleepSetting(pmsetOutput string) (minutes int, note string, ok bool) {
	m := sleepLinePattern.FindStringSubmatch(pmsetOutput)
	if m == nil {
		return 0, "", false
	}
	value, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, "", false
	}
	return value, m[2], true
}
