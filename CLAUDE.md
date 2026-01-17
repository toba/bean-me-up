# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build
go build .

# Run tests
go test ./...

# Run single test
go test ./internal/clickup -run TestFunctionName

# Lint (required before commits)
golangci-lint run

# Install locally
go install .
```

## Architecture Overview

bean-me-up syncs [beans](https://github.com/hmans/beans) issue tracker to ClickUp tasks. It operates as a standalone companion that:
- Invokes the `beans` CLI with `--json` output (no library dependency)
- Stores sync state in `.beans/.sync.json` (separate from bean files)
- Syncs to ClickUp via REST API

### Package Structure

| Package | Purpose |
|---------|---------|
| `cmd/` | Cobra CLI commands. Each command is a file; register with `rootCmd.AddCommand()` in `init()` |
| `internal/config/` | YAML configuration loading with default mappings |
| `internal/beans/` | Wrapper around beans CLI, JSON parsing |
| `internal/clickup/` | REST API client with retry logic, sync orchestration |
| `internal/syncstate/` | Manages sync state in `.beans/.sync.json` |
| `internal/frontmatter/` | Parses/writes YAML frontmatter in bean markdown files (legacy) |

### Sync Flow

The sync logic (`internal/clickup/sync.go`) uses a multi-pass approach:
1. **Pass 1**: Sync parent tasks (beans without parents or parents not in sync set)
2. **Pass 2**: Sync child tasks (with parent references now available)
3. **Pass 3**: Sync blocking relationships as ClickUp dependencies

Processing is parallelized with goroutines and `sync.WaitGroup`.

### Sync State Storage

Sync metadata is stored in `.beans/.sync.json` (managed by `internal/syncstate/`):
```json
{
  "version": 1,
  "beans": {
    "bean-abc1": {
      "clickup": {
        "task_id": "868habcd",
        "synced_at": "2024-01-15T10:30:00Z"
      }
    }
  }
}
```

This approach avoids the problem of the beans CLI overwriting frontmatter when it updates bean files.

### API Retry Logic

The ClickUp client (`internal/clickup/client.go`) implements exponential backoff with jitter for rate limits (429, APP_002), transient network errors, and 5xx responses. Max 5 retries, max 30s delay.

## Configuration

Requires `.bean-me-up.yml` (found via upward search from cwd) and `CLICKUP_TOKEN` environment variable.

See `.bean-me-up.yml.example` for all options.
