package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// noInput is the argument type for tools that take none. The SDK requires a
// struct or map so the inferred schema is an object, as the spec demands.
type noInput struct{}

// registerTools exposes the service over MCP.
//
// Every handler is a type adapter and nothing more: unpack, call one Service
// method, hand back the result. No validation, no error special-casing, no
// decisions. That is what makes these tools behave identically to the CLI
// rather than merely similarly — they are the same implementation reached two
// ways, not two implementations kept in step.
//
// Deliberately absent: install and uninstall. Registering this server as an
// MCP server from inside itself would require it to already be registered and
// running, which is a bootstrapping problem, not a feature.
func registerTools(server *mcp.Server, svc *Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_projects",
		Description: "List the projects available to start an environment for. " +
			"Reads the cache written by the last scan and never scans itself, so every " +
			"caller sees the same list; use rescan_projects to refresh it.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, *ProjectCache, error) {
		cache, err := svc.ListProjects()
		return nil, cache, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "rescan_projects",
		Description: "Rescan the configured directories for projects and refresh the cache. " +
			"Reports what changed, which directories were skipped and why, and any unusable " +
			"configuration patterns.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, *RescanResult, error) {
		result, err := svc.Rescan()
		return nil, result, err
	})

	type environmentList struct {
		Environments []Environment `json:"environments"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_environments",
		Description: "List the remote-control environments currently running, with the URL to " +
			"open each one. Records whose process has died are dropped rather than reported.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, *environmentList, error) {
		environments, err := svc.ListEnvironments()
		if err != nil {
			return nil, nil, err
		}
		return nil, &environmentList{Environments: environments}, nil
	})

	type startInput struct {
		ProjectName string `json:"project_name,omitempty" jsonschema:"name of a project from list_projects; give this or project_path, not both"`
		ProjectPath string `json:"project_path,omitempty" jsonschema:"absolute path to a directory; works for directories the project scan did not find"`
		SpawnMode   string `json:"spawn_mode,omitempty" jsonschema:"same-dir or worktree; omit to use the configured default"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "start_environment",
		Description: "Start a Claude Code remote-control environment for a project and return the " +
			"URL to open it on another device. One environment serves one directory and hosts up " +
			"to 32 sessions. If one is already running for the project, it is returned as-is with " +
			"already_running set — starting twice reconnects rather than duplicating.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in startInput) (*mcp.CallToolResult, *StartResult, error) {
		result, err := svc.StartEnvironment(StartArgs{
			Target:    ProjectTarget{Name: in.ProjectName, Path: in.ProjectPath},
			SpawnMode: SpawnMode(in.SpawnMode),
		})
		return nil, result, err
	})

	type stopInput struct {
		ProjectName string `json:"project_name,omitempty" jsonschema:"name of a project from list_projects; give this or project_path, not both"`
		ProjectPath string `json:"project_path,omitempty" jsonschema:"absolute path to the project directory"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "stop_environment",
		Description: "Stop the remote-control environment running for a project. Sessions inside it " +
			"end with it, though claude preserves the environment itself and a later start reconnects.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in stopInput) (*mcp.CallToolResult, *StopResult, error) {
		result, err := svc.StopEnvironment(ProjectTarget{Name: in.ProjectName, Path: in.ProjectPath})
		return nil, result, err
	})
}

// serveMCP runs the server over stdio until the client disconnects.
func serveMCP(ctx context.Context, svc *Service) error {
	server := mcp.NewServer(&mcp.Implementation{Name: appName, Version: version}, nil)
	registerTools(server, svc)
	return server.Run(ctx, &mcp.StdioTransport{})
}
