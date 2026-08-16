//go:build darwin

package main

import "testing"

// Fixtures below are real pmset -g output, captured on this machine — one
// while a caffeinate assertion was active, one from the sleep-allowed
// default. Real output rather than an imagined shape, for the same reason
// the claude URL pattern is tested against a captured banner: the format is
// not a documented contract, so a guessed shape proves nothing about the
// real one.

const pmsetSleepPreventedByCaffeinate = `System-wide power settings:
Currently in use:
 standby              1
 Sleep On Power Button 1
 hibernatefile        /var/vm/sleepimage
 powernap             1
 networkoversleep     0
 disksleep            10
 sleep                0 (sleep prevented by caffeinate, caffeinate)
 hibernatemode        3
 ttyskeepawake        1
 displaysleep         10
 tcpkeepalive         1
 powermode            0
 womp                 1
`

const pmsetSleepAllowedAfter10Min = `System-wide power settings:
Currently in use:
 standby              1
 hibernatefile        /var/vm/sleepimage
 powernap             1
 networkoversleep     0
 disksleep            10
 sleep                10
 hibernatemode        3
 displaysleep         10
 womp                 1
`

const pmsetSleepNeverConfigured = `System-wide power settings:
Currently in use:
 sleep                0
 displaysleep         10
 disksleep            10
`

func TestParseSleepSetting(t *testing.T) {
	t.Run("prevented by an active assertion", func(t *testing.T) {
		minutes, note, ok := parseSleepSetting(pmsetSleepPreventedByCaffeinate)
		if !ok {
			t.Fatal("expected the sleep line to be found")
		}
		if minutes != 0 {
			t.Errorf("minutes = %d, want 0", minutes)
		}
		if note != "sleep prevented by caffeinate, caffeinate" {
			t.Errorf("note = %q", note)
		}
	})

	t.Run("configured to sleep after a delay", func(t *testing.T) {
		minutes, note, ok := parseSleepSetting(pmsetSleepAllowedAfter10Min)
		if !ok {
			t.Fatal("expected the sleep line to be found")
		}
		if minutes != 10 {
			t.Errorf("minutes = %d, want 10", minutes)
		}
		if note != "" {
			t.Errorf("note = %q, want none", note)
		}
	})

	t.Run("never sleeps, no assertion needed", func(t *testing.T) {
		minutes, _, ok := parseSleepSetting(pmsetSleepNeverConfigured)
		if !ok {
			t.Fatal("expected the sleep line to be found")
		}
		if minutes != 0 {
			t.Errorf("minutes = %d, want 0", minutes)
		}
	})

	t.Run("does not confuse disksleep, displaysleep or networkoversleep with sleep", func(t *testing.T) {
		// Every fixture above also contains these lines. If the pattern were
		// not anchored to the start of the line, disksleep's "10" could be
		// mistaken for the value that matters.
		out := "System-wide power settings:\n" +
			"Currently in use:\n" +
			" networkoversleep     0\n" +
			" disksleep            10\n" +
			" displaysleep         10\n"
		if _, _, ok := parseSleepSetting(out); ok {
			t.Error("expected no match when the sleep key itself is absent")
		}
	})

	t.Run("unparseable output is reported honestly", func(t *testing.T) {
		if _, _, ok := parseSleepSetting("nothing resembling pmset output"); ok {
			t.Error("expected no match")
		}
	})
}

func TestCheckSystemSleep(t *testing.T) {
	// The command itself is not stubbed — pmset is a fixed system tool, not
	// something this program's configuration can point elsewhere, unlike
	// claudeBin. This just confirms the check runs on the machine it's
	// actually built for and returns something.
	check, ok := checkSystemSleep()
	if !ok {
		t.Fatal("expected a check on darwin")
	}
	if check.Name != "system sleep" {
		t.Errorf("name = %q", check.Name)
	}
	if check.Status == StatusFail {
		t.Error("this check must never fail; whether sleep is allowed is the user's call")
	}
}
