package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ProjectCache is the result of the last rescan.
//
// list-projects reads only this file and never scans live. Without that split,
// the CLI and the MCP server could each see a different filesystem state
// depending on when they happened to run; the cache is the shared view both
// are guaranteed to read.
type ProjectCache struct {
	SchemaVersion int       `json:"schema_version"`
	ScannedAt     time.Time `json:"scanned_at"`
	Projects      []Project `json:"projects"`
}

func loadProjectCache(path string) (*ProjectCache, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// No rescan has run yet. An empty cache is the honest answer.
		return &ProjectCache{SchemaVersion: stateSchemaVersion}, nil
	}
	if err != nil {
		return nil, err
	}

	var cache ProjectCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Regenerating this file is one command, so an unreadable version is a
	// nuisance rather than a loss.
	if err := checkSchemaVersion(path, cache.SchemaVersion, "run '"+appName+" rescan'"); err != nil {
		return nil, err
	}
	return &cache, nil
}

func saveProjectCache(path string, cache *ProjectCache) error {
	cache.SchemaVersion = stateSchemaVersion

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}
