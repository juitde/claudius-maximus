package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// defaultURLPattern extracts the environment URL from claude's output.
//
// Verified against claude 2.1.233, which prints:
//
//	Continue coding in the Claude mobile app or https://claude.ai/code?environment=env_01LB1RY…
//
// Two things about it are easy to get wrong. The identifier is a query
// parameter, not a path segment — a pattern expecting /code/<id> never matches
// at all. And the same output carries a second link,
// https://code.claude.com/docs/en/remote-control, so a loose "any URL" match
// happily reports the documentation and declares success before the process has
// connected to anything.
//
// The capture group is the environment ID, which claude derives from the
// directory and keeps stable across restarts.
const defaultURLPattern = `https://claude\.ai/code\?environment=(env_[A-Za-z0-9]+)`

// envURLPattern overrides the pattern without needing a new build. This parses
// human-facing CLI output, which is not a contract; when it changes, an
// override is the difference between a config edit and waiting for a release.
const envURLPattern = envPrefix + "URL_PATTERN"

func resolveURLPattern() (*regexp.Regexp, error) {
	pattern := os.Getenv(envURLPattern)
	if pattern == "" {
		return regexp.MustCompile(defaultURLPattern), nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s is not a valid regular expression: %w", envURLPattern, err)
	}
	if compiled.NumSubexp() < 1 {
		return nil, fmt.Errorf("%s must contain a capture group for the environment ID", envURLPattern)
	}
	return compiled, nil
}

// spawnSpec is everything needed to start one remote-control process.
type spawnSpec struct {
	ProjectPath string
	ClaudeBin   string
	LogPath     string
	SpawnMode   SpawnMode
	Timeout     time.Duration
}

// spawnOutcome is what could be learned about the process that was started.
type spawnOutcome struct {
	PID           int
	URL           string
	EnvironmentID string
}

const defaultSpawnTimeout = 60 * time.Second

// claudeArgs builds the command line.
//
// --spawn is always passed. Left off, claude asks which spawn mode to use as
// soon as it has a terminal, and waits. Without a terminal it silently
// defaults, so the prompt only appears on exactly the path this tool prefers —
// inside tmux — where nobody is watching to answer it.
func claudeArgs(mode SpawnMode) []string {
	return []string{"remote-control", "--spawn=" + string(mode)}
}

// spawnPlain starts claude detached, with its output redirected to a log file,
// and waits for the environment URL to appear.
//
// This is the fallback for machines with no multiplexer: it works, but nothing
// can attach to the running process afterwards.
func spawnPlain(spec spawnSpec) (*spawnOutcome, error) {
	pattern, err := resolveURLPattern()
	if err != nil {
		return nil, err
	}

	logFile, err := os.Create(spec.LogPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(spec.ClaudeBin, claudeArgs(spec.SpawnMode)...)
	cmd.Dir = spec.ProjectPath
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setDetached(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.ClaudeBin, err)
	}
	pid := cmd.Process.Pid

	// Let the process go rather than reaping it: it has to outlive whichever
	// of our entry points started it.
	cmd.Process.Release()

	url, environmentID, err := awaitURL(spec.LogPath, pattern, spec.Timeout, func() bool {
		return processAlive(pid)
	})
	if err != nil {
		return nil, err
	}
	return &spawnOutcome{PID: pid, URL: url, EnvironmentID: environmentID}, nil
}

// awaitURL polls the log until the pattern matches, the process dies, or the
// timeout expires.
//
// stillRunning lets a process that exits early be reported immediately with
// what it printed, instead of making the user wait out the full timeout for a
// message that says nothing.
func awaitURL(logPath string, pattern *regexp.Regexp, timeout time.Duration, stillRunning func() bool) (url, environmentID string, err error) {
	if timeout <= 0 {
		timeout = defaultSpawnTimeout
	}
	deadline := time.Now().Add(timeout)

	for {
		if url, environmentID, found := matchLog(logPath, pattern); found {
			return url, environmentID, nil
		}
		if !stillRunning() {
			return "", "", fmt.Errorf("claude exited before reporting an environment URL:\n%s", logTail(logPath))
		}
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf(
				"no environment URL after %s. claude may be waiting for input, or its output format may have changed "+
					"(override the pattern with %s). Last output:\n%s",
				timeout, envURLPattern, logTail(logPath))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func matchLog(logPath string, pattern *regexp.Regexp) (url, environmentID string, found bool) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", "", false
	}
	// The log is a redrawing TUI, so the same URL appears in every repainted
	// frame. The first match is as good as any.
	m := pattern.FindSubmatch(data)
	if m == nil {
		return "", "", false
	}
	return string(m[0]), string(m[1]), true
}

// logTailLines is how much of the log an error message carries — enough to
// show a prompt or an authentication failure, short enough to read.
const logTailLines = 12

// logTail returns the end of the log with terminal control sequences removed.
//
// Stripping matters: claude repaints its whole display, so a raw tail is
// mostly cursor movement, and an error message full of escape codes is worse
// than no error message.
func logTail(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "  (no output captured)"
	}

	var kept []string
	for _, line := range strings.Split(stripANSI(string(data)), "\n") {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, "  "+strings.TrimRight(line, " \t"))
		}
	}
	if len(kept) == 0 {
		return "  (no output captured)"
	}
	if len(kept) > logTailLines {
		kept = kept[len(kept)-logTailLines:]
	}
	return strings.Join(kept, "\n")
}

// ansiPattern covers the escape sequences claude actually emits: CSI sequences
// such as cursor-up and erase-display, plus the two-character escapes that can
// appear between them.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b[()][A-Za-z0-9]|\x1b[=>]|\r`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
