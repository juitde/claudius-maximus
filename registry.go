package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Multiplexer is where a remote-control process runs.
//
// The spawning strategies live with the multiplexer code; this type is here
// because it is part of the persisted record — what a stored environment needs
// is the knowledge of how to check on it and how to shut it down.
type Multiplexer string

const (
	MuxNone   Multiplexer = "none"
	MuxTmux   Multiplexer = "tmux"
	MuxScreen Multiplexer = "screen"
)

// SpawnMode is the --spawn value a remote-control process was started with.
//
// It has to be passed explicitly. Asked on a terminal and left unanswered,
// claude blocks on an interactive prompt, which means anything started inside
// tmux or screen would hang forever.
type SpawnMode string

const (
	SpawnSameDir  SpawnMode = "same-dir"
	SpawnWorktree SpawnMode = "worktree"
)

// defaultSpawnMode is same-dir, matching claude's own default and, more to the
// point, the only mode that works everywhere.
//
// A worktree contains only what is committed. Everything a project needs to
// actually run but does not track — vendor directories, node_modules, .env
// files, local database state — is absent, so a worktree session can read and
// write code but not execute anything. It also requires a git repository,
// which project discovery deliberately does not insist on.
//
// worktree earns its place when several sessions must work on one project at
// once without colliding; that is a deliberate choice, not a default.
const defaultSpawnMode = SpawnSameDir

// Environment is one running `claude remote-control` process.
//
// The name follows what claude actually reports: a process is an *environment*
// bound to a directory, hosting up to 32 sessions that the mobile app creates
// inside it. It is not itself a session, and modelling it as one would make
// the counting wrong in both directions.
type Environment struct {
	// ProjectPath is the identity of the record. claude derives a stable
	// environment per directory — starting twice in the same place reconnects
	// rather than creating a second one — so the path is the natural key and a
	// self-issued ID would only invent a distinction that does not exist.
	ProjectPath string `json:"project_path"`

	// ProjectName is bookkeeping copied from the project cache, kept current
	// by Rescan. Only for display; the path is what identifies anything.
	ProjectName string `json:"project_name"`

	// EnvironmentID is claude's own identifier, parsed from its output.
	EnvironmentID string `json:"environment_id"`
	URL           string `json:"url"`

	PID         int         `json:"pid"`
	StartedAt   time.Time   `json:"started_at"`
	LogFile     string      `json:"log_file"`
	Multiplexer Multiplexer `json:"multiplexer"`
	MuxName     string      `json:"mux_name,omitempty"`
	SpawnMode   SpawnMode   `json:"spawn_mode,omitempty"`
}

// RenameEvent records a project name that changed under a running environment.
type RenameEvent struct {
	ProjectPath string `json:"project_path"`
	OldName     string `json:"old_name"`
	NewName     string `json:"new_name"`
}

// Registry is the persisted list of running environments.
//
// Known limitation: the mutex guards concurrency inside this process only. Two
// processes — the CLI and the MCP server, say — writing the same file at once
// are not protected. Real file locking (flock / LockFileEx) is the fix, and is
// deliberately deferred: the window is small, the writes are rare, and the
// atomic replace means a loser overwrites rather than corrupts.
type Registry struct {
	path string
	mu   sync.Mutex
}

func newRegistry(stateDir string) *Registry {
	return &Registry{path: stateFile(stateDir, "environments.json")}
}

func (r *Registry) load() ([]Environment, error) {
	data, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var environments []Environment
	if err := json.Unmarshal(data, &environments); err != nil {
		return nil, fmt.Errorf("parse %s: %w", r.path, err)
	}
	normalize(environments)
	return environments, nil
}

// normalize fills in fields that a hand-edited or older file may leave empty,
// before any other code sees the records.
//
// Idempotent, so no version marker is needed: running it twice changes
// nothing, and a version field would be complexity without a payoff.
func normalize(environments []Environment) {
	for i := range environments {
		if environments[i].Multiplexer == "" {
			environments[i].Multiplexer = MuxNone
		}
	}
}

func (r *Registry) save(environments []Environment) error {
	data, err := json.MarshalIndent(environments, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(r.path, append(data, '\n'), 0o600)
}

// Put stores an environment, replacing any existing record for the same path.
//
// An upsert rather than an append, because a second start against the same
// directory is a reconnect to the same environment. Appending would leave two
// records claiming one environment, with the stale one holding a dead PID.
func (r *Registry) Put(env Environment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	environments, err := r.load()
	if err != nil {
		return err
	}
	for i := range environments {
		if environments[i].ProjectPath == env.ProjectPath {
			environments[i] = env
			return r.save(environments)
		}
	}
	return r.save(append(environments, env))
}

// Get returns the environment recorded for a path, if any.
func (r *Registry) Get(projectPath string) (*Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	environments, err := r.load()
	if err != nil {
		return nil, err
	}
	for i := range environments {
		if environments[i].ProjectPath == projectPath {
			found := environments[i]
			return &found, nil
		}
	}
	return nil, nil
}

// Remove drops the record for a path, reporting whether there was one.
func (r *Registry) Remove(projectPath string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	environments, err := r.load()
	if err != nil {
		return false, err
	}
	kept := make([]Environment, 0, len(environments))
	for _, env := range environments {
		if env.ProjectPath != projectPath {
			kept = append(kept, env)
		}
	}
	if len(kept) == len(environments) {
		return false, nil
	}
	return true, r.save(kept)
}

// List returns every record, without checking whether any still runs.
func (r *Registry) List() ([]Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.load()
}

// ListAlive returns the environments that isAlive accepts, and drops the rest
// from the file as a side effect.
//
// The liveness check is injected rather than called directly: what "alive"
// means depends on the multiplexer, and taking it as a parameter is also what
// makes this testable without spawning real processes.
func (r *Registry) ListAlive(isAlive func(Environment) bool) ([]Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	environments, err := r.load()
	if err != nil {
		return nil, err
	}

	alive := make([]Environment, 0, len(environments))
	for _, env := range environments {
		if isAlive(env) {
			alive = append(alive, env)
		}
	}
	if len(alive) != len(environments) {
		if err := r.save(alive); err != nil {
			return nil, err
		}
	}
	return alive, nil
}

// RenameByPath updates the cached project name of stored environments.
//
// Only this tool's bookkeeping changes. The running process and whatever
// claude.ai shows are untouched — a project name is our label for a directory,
// not something the environment knows about itself.
func (r *Registry) RenameByPath(names map[string]string) ([]RenameEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	environments, err := r.load()
	if err != nil {
		return nil, err
	}

	var events []RenameEvent
	for i := range environments {
		newName, known := names[environments[i].ProjectPath]
		if !known || newName == environments[i].ProjectName {
			continue
		}
		events = append(events, RenameEvent{
			ProjectPath: environments[i].ProjectPath,
			OldName:     environments[i].ProjectName,
			NewName:     newName,
		})
		environments[i].ProjectName = newName
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, r.save(environments)
}
