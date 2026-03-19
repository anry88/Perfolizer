# pkg/agent

This package owns the Perfolizer execution agent.

## Responsibilities

- Accept serialized plans over HTTP.
- Execute enabled top-level thread groups.
- Aggregate and expose Prometheus metrics.
- Expose host metrics for UI runtime views.
- Provide HTTP debug execution for request-level inspection.
- Optionally execute an admin restart command.

## Key Files

- `server.go`: main HTTP server, endpoint handlers, metrics rendering, restart handling.
- `host_metrics.go`: shared host metrics structures and collection plumbing.
- `host_metrics_linux.go`: Linux host metric collection.
- `host_metrics_darwin.go`: macOS host metric collection.
- `host_metrics_windows.go`: Windows host metric collection.
- `host_metrics_other.go`: fallback implementation for unsupported platforms.

## Current HTTP Surfaces

- `POST /run`
- `POST /stop`
- `GET /metrics`
- `GET /healthz`
- `POST /debug/http`
- `POST /admin/restart`
- `GET /favicon.ico`

## Things To Keep In Sync

- If metric names or labels change, update `pkg/ui/agent_client.go`.
- If restart behavior changes, update `README.md` and the agent settings UI docs.
- If plan parsing or execution shape changes, validate `pkg/core` persistence compatibility.

## Safety Notes

- `/admin/restart` is disabled by default.
- The restart command is executed by the agent process, so changes here are security-sensitive.
- Keep token handling and command execution paths narrow and explicit.
