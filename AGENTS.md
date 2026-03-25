# Perfolizer Agent Guide

This repository is a desktop-first load testing product written in Go. The core split is:

- `cmd/perfolizer`: native Fyne desktop UI.
- `cmd/agent`: execution agent with HTTP control surfaces and Prometheus metrics.
- `pkg/core`: plan model, persistence, runtime context, stats.
- `pkg/elements`: samplers, thread groups, controllers.
- `pkg/ui`: editor, settings, dashboard, agent client, screenshot generator support.
- `pkg/agent`: HTTP server, metrics rendering, debug HTTP, restart handling.
- `pkg/config`: shared config loading used by both UI and agent.

## First Pass For Any Agent

1. Read [README.md](README.md) for product positioning and operator-facing behavior.
2. Read the package README for the area you are about to change:
   - [cmd/README.md](cmd/README.md)
   - [pkg/README.md](pkg/README.md)
   - [tests/README.md](tests/README.md)
3. If the task touches an element type, inspect both `pkg/elements` and `pkg/ui/app.go`.
4. If the task touches agent behavior, inspect both `pkg/agent/server.go` and `pkg/ui/agent_client.go`.

## High-Value Commands

Run tests:

```bash
GOCACHE="$PWD/.cache/go-build" go test ./...
```

Generate README screenshots:

```bash
GOCACHE="$PWD/.cache/go-build" go run ./cmd/generate_screenshots
```

Build the main entrypoints:

```bash
GOCACHE="$PWD/.cache/go-build" go build ./cmd/agent ./cmd/perfolizer ./cmd/generate_screenshots
```

Run locally:

```bash
go run ./cmd/agent
go run ./cmd/perfolizer
```

## Repository Rules That Matter

- The UI and agent are intentionally separate processes. Do not collapse them into one runtime unless that is the explicit task.
- `config/agent.json` is shared by the UI and the agent for default connectivity behavior.
- Public README claims should stay grounded in shipped behavior. Avoid benchmark or feature claims that the code does not support.
- The screenshot assets in `docs/screenshots/` are generated, not hand-maintained.
- Current multi-agent UI is agent management and switching. Do not describe it as distributed orchestration unless that is implemented.
- If code changes were made while on a main branch (for example `main` or `master`), create a new branch before committing or pushing those changes.

## Common Change Paths

### Add or modify a test element

You usually need to touch all of these:

- `pkg/elements/*`: implementation, serialization props, factory registration.
- `pkg/ui/app.go`: element palette and property form rendering.
- `tests/core` or `tests/elements`: persistence/behavior coverage.

### Change agent API or metrics

You usually need to touch all of these:

- `pkg/agent/server.go`: endpoint or metric emission.
- `pkg/ui/agent_client.go`: parsing/client behavior.
- `README.md`: runtime surface docs.
- `tests/*`: add parsing or config coverage where relevant.

### Change project persistence

Read first:

- `pkg/core/persistence.go`
- `pkg/core/project.go`
- `tests/core/persistence_test.go`

Backward compatibility matters because the UI can still load legacy single-plan JSON files.

## Useful Package Maps

- [cmd/README.md](cmd/README.md): entrypoints and generation utilities.
- [pkg/README.md](pkg/README.md): package-level map.
- [pkg/ui/README.md](pkg/ui/README.md): UI ownership and extension points.
- [pkg/agent/README.md](pkg/agent/README.md): agent endpoints and metrics.
- [pkg/core/README.md](pkg/core/README.md): model, persistence, stats, context.
- [pkg/elements/README.md](pkg/elements/README.md): supported element types and extension rules.
- [pkg/config/README.md](pkg/config/README.md): config loading and derived addresses.
- [tests/README.md](tests/README.md): test layout and where to add coverage.
