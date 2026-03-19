# pkg/config

This package owns agent/UI connectivity configuration.

## What It Does

- Loads `config/agent.json` by default.
- Supports `PERFOLIZER_AGENT_CONFIG` override.
- Applies defaults for host, port, and UI poll interval.
- Derives the address the agent should bind to.
- Derives the base URL the UI should use for the default local agent.

## Key File

- `agent.go`

## Important Behavior

- If `listen_host` is empty, it defaults to `127.0.0.1`.
- If `port` is empty, it defaults to `9090`.
- If `ui_poll_interval_seconds` is empty, it defaults to `5`.
- If `listen_host` is `0.0.0.0` and `ui_connect_host` is not set, the UI falls back to `127.0.0.1` for the default base URL.

## Why This Matters

The same config model is used by:

- `cmd/agent` for server startup.
- `pkg/ui/agent_client.go` and UI startup for default local connectivity.

If you change derived address behavior here, verify both the agent and the UI startup flows.
