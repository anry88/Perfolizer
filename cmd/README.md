# cmd

This directory contains executable entrypoints and repo utilities.

## Entrypoints

- `agent`: starts the Perfolizer execution agent.
- `perfolizer`: starts the native desktop UI.
- `generate_icon`: icon-generation utility for build assets.
- `generate_screenshots`: deterministic showcase screenshot generator used for README assets.

## What Each Command Owns

### `cmd/agent`

- Loads config via `pkg/config`.
- Builds `pkg/agent.Server`.
- Starts the HTTP server for `/run`, `/stop`, `/metrics`, `/debug/http`, `/healthz`, and optional `/admin/restart`.

### `cmd/perfolizer`

- Boots the Fyne app from `pkg/ui`.
- Owns no business logic beyond constructing and running the UI.

### `cmd/generate_screenshots`

- Uses a Fyne test app to render stable screenshots into `docs/screenshots/`.
- Safe place to update README visual fixtures without changing product behavior.

## When To Edit This Directory

- Add a new executable entrypoint.
- Change startup wiring between packages.
- Add a repo utility that is useful to contributors or coding agents.

If the change is product behavior rather than process bootstrap, the real work probably belongs under `pkg/`.
