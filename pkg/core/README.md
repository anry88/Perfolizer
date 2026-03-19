# pkg/core

This package contains the shared model and runtime infrastructure used by both the UI and the agent.

## Responsibilities

- Core element interfaces.
- Base tree element implementation.
- Project and plan containers.
- JSON persistence for projects and test plans.
- Thread execution context and variable substitution.
- Runtime statistics aggregation.
- Debug HTTP transport models.

## Key Files

- `interfaces.go`: `TestElement`, `Executable`, `ThreadGroup`, `BaseElement`, ID generation.
- `project.go`: multi-plan project container.
- `persistence.go`: JSON read/write, DTO mapping, factory-based rehydration.
- `context.go`: runtime variables, parameter definitions, substitution logic.
- `stats.go`: `StatsRunner` and aggregated metrics snapshots.
- `parameter.go`: plan parameter types and extractor helpers.
- `debug_http.go`: request/response structs used by debug HTTP flows.

## Persistence Model

There are two related JSON shapes:

- Project JSON: one file containing multiple named plans and plan-level parameters.
- Test plan JSON: a single root test plan tree.

The UI prefers project files, but it still supports loading legacy single-plan JSON.

## Important Constraints

- New element types must register a factory, expose serializable props, and round-trip through `persistence.go`.
- Variable substitution is string-based and powered by the runtime `Context`.
- `StatsRunner` publishes interval metrics and keeps cumulative totals.

## When To Edit This Package

- Add a new cross-cutting runtime concept.
- Change project or plan persistence.
- Extend the runtime context or metric model.
- Add a new element interface contract used by both UI and agent.
