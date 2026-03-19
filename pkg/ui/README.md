# pkg/ui

This package owns the native desktop application built with Fyne.

## Responsibilities

- Test plan tree editor and property panels.
- Parameter management UI.
- Agent settings and runtime status views.
- Runtime dashboard windows.
- Debug request/extraction workflow.
- HTTP client for talking to the agent.
- Showcase screenshot generation support.

## Key Files

- `app.go`: main editor window, tree, toolbar, property forms, run/debug flows.
- `settings.go`: settings window, agent management, runtime cards, host metrics display.
- `agent_client.go`: HTTP client and Prometheus metric parsing.
- `dashboard.go`: runtime charts window.
- `chart.go`: custom line chart widget.
- `param_manager.go`: plan parameter editor.
- `showcase_screenshots.go`: deterministic screenshot fixtures for the repo README.

## Important Flows

### Normal run

1. Resolve active agent from preferences/config.
2. Serialize the current plan.
3. Send it to `POST /run`.
4. Open a dashboard window.
5. Poll `/metrics` on the selected agent.

### Debug run

1. Collect HTTP samplers from the current plan.
2. Call `POST /debug/http` once per sampler.
3. Show request/response details in the debug console.
4. Reuse plan parameters for extraction testing.

### Agent settings

1. Load saved agents from Fyne preferences.
2. Build `AgentClient` instances per agent.
3. Poll `/metrics` to derive runtime and host state.
4. Optionally call `/admin/restart` when enabled.

## Extension Notes

- Adding a new element type is not only a `pkg/elements` change. Update the component lists and property rendering in `app.go`.
- If you change metric names or labels in the agent, update `agent_client.go` parsing in the same change.
- This package assumes JSON-backed plan persistence from `pkg/core`, so new editable fields should also be serializable.
- Screenshot content is synthetic and should stay free of secrets, internal hostnames, or unsafe restart examples.
