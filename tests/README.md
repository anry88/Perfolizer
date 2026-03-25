# tests

This directory contains external-package tests grouped by domain.

## Layout

- `agent/`: agent HTTP surface and runtime validation behavior.
- `ai/`: AI draft generation, patch validation, and rules-first authoring coverage.
- `config/`: config loading and derived address behavior.
- `core/`: project model, persistence, context, stats, helpers, and interfaces.
- `elements/`: element-level helpers and extraction behavior.

## Why External-Package Tests

Most tests use `*_test` packages to exercise public behavior instead of package internals. That makes them useful as compatibility checks when refactoring.

## Where To Add Coverage

- Add agent API and runtime validation coverage under `tests/agent`.
- Add AI provider, draft, or patch behavior coverage under `tests/ai`.
- Add config parsing or defaulting tests under `tests/config`.
- Add persistence, runtime model, or stats coverage under `tests/core`.
- Add sampler/controller/thread-group helper coverage under `tests/elements`.

## Useful Command

```bash
GOCACHE="$PWD/.cache/go-build" go test ./...
```
