# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build
go build ./cmd/beanup

# Run tests
go test ./...

# Run single test
go test ./internal/clickup -run TestFunctionName

# Lint (required before commits)
golangci-lint run

# Install locally
go install ./cmd/beanup
```

## Architecture Overview

bean-me-up syncs [beans](https://github.com/hmans/beans) issue tracker to ClickUp tasks. It operates as a standalone companion that:
- Invokes the `beans` CLI with `--json` output (no library dependency)
- Stores sync state in bean extension metadata
- Syncs to ClickUp via REST API

### Package Structure

| Package | Purpose |
|---------|---------|
| `cmd/` | Cobra CLI commands. Each command is a file; register with `rootCmd.AddCommand()` in `init()` |
| `cmd/beanup/` | Main entrypoint for the `beanup` binary |
| `internal/config/` | YAML configuration loading with default mappings |
| `internal/beans/` | Wrapper around beans CLI, JSON parsing |
| `internal/clickup/` | REST API client with retry logic, sync orchestration, `ExtensionSyncProvider` |
| `internal/syncstate/` | Legacy sync state in `.beans/.sync.json` (used only by `migrate` command) |
| `internal/frontmatter/` | Parses/writes YAML frontmatter in bean markdown files (legacy) |

### Sync Flow

The sync logic (`internal/clickup/sync.go`) uses a multi-pass approach:
1. **Pass 1**: Sync parent tasks (beans without parents or parents not in sync set)
2. **Pass 2**: Sync child tasks (with parent references now available)
3. **Pass 3**: Sync blocking relationships as ClickUp dependencies

Processing is parallelized with goroutines and `sync.WaitGroup`.

### Sync State Storage

Sync metadata is stored in each bean's extension metadata. During sync, an `ExtensionSyncProvider` (in `internal/clickup/external_sync.go`) reads extension data from beans at startup, caches it in memory, and flushes all changes as a single batched `beans query` call at the end. This means only 2 `beans` CLI invocations per sync regardless of bean count.

The `SyncStateProvider` interface abstracts sync state access so the `Syncer` doesn't depend on any specific storage backend.

### API Retry Logic

The ClickUp client (`internal/clickup/client.go`) implements exponential backoff with jitter for rate limits (429, APP_002), transient network errors, and 5xx responses. Max 5 retries, max 30s delay.

## Configuration

Configuration is stored in the `extensions.clickup` section of `.beans.yml`, with fallback to legacy `.beans.clickup.yml`. Requires `CLICKUP_TOKEN` environment variable.

See `.beans.clickup.yml.example` for all config options.
