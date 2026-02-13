# Perfolizer

Perfolizer is a lightweight, JMeter-like load testing tool written in Golang. It provides a native GUI for creating and running performance tests using a tree-based structure, similar to JMeter, but powered by the efficiency of Go.

## Architecture

- **UI process (`cmd/perfolizer`)**: builds/edits test plans and visualizes runtime metrics.
- **Agent process (`cmd/agent`)**: executes tests in a separate process and exposes Prometheus metrics.
- **Decoupling**: UI remains responsive and independent from test execution lifecycle.

## Features

-   **Pure Golang**: Built with Go for high performance and easy compilation.
-   **Visual Test Plans**: Create test scenarios using a GUI with a tree structure.
-   **Code-First Support**: (Planned) Define scenarios directly in Go code.
-   **Load Generators**:
    -   **Simple Thread Group**: Fixed number of users and iterations.
    -   **RPS Thread Group**: Target Requests Per Second with concurrent workers.
-   **Samplers**:
    -   **HTTP Sampler**: Perform GET/POST/PUT/DELETE requests.
-   **Logic Controllers**:
    -   **Loop Controller**: Repeat actions a specific number of times.
    -   **If Controller**: Conditionally execute children.
    -   **Pause Controller**: Add delays between actions.
-   **Live Monitoring**: (In Progress) View test progress and structure.
-   **Remote Agent Execution**: Test plans are sent from UI to a separate agent process.
-   **Prometheus Metrics**: Agent exposes `/metrics` with RPS, latency, errors, and totals.
-   **Polling Dashboard**: UI fetches agent metrics every 15 seconds and updates charts.

## Prerequisites

-   **Go 1.20+**: Ensure Go is installed and available in your PATH.
-   **Fyne Dependencies**: You may need C compilers for Fyne (usually `gcc` on Linux/Mac, or Mingw on Windows) if not already present, though pure Go modules often suffice for basic builds.

## Installation & Setup

1.  **Clone the repository**:
    ```bash
    git clone git@github.com:anry88/Perfolizer.git
    cd perfolizer
    ```

2.  **Initialize dependencies** (if not already done):
    ```bash
    go mod init perfolizer
    go mod tidy
    ```
    *Note: This project uses `fyne.io/fyne/v2` for the UI and `golang.org/x/time/rate` for rate limiting.*

3.  **Install dependencies**:
    ```bash
    go get fyne.io/fyne/v2
    go get golang.org/x/time/rate
    ```

## Usage

### Running the GUI

1. Start the agent in a separate process:
   ```bash
   go run cmd/agent/main.go
   ```

2. Start the GUI:
   ```bash
   go run cmd/perfolizer/main.go
   ```

### Agent Configuration

Default config file: `config/agent.json`

```json
{
  "listen_host": "127.0.0.1",
  "port": 9090,
  "ui_poll_interval_seconds": 15,
  "enable_remote_restart": false,
  "remote_restart_token": "",
  "remote_restart_command": ""
}
```

You can override config path with:
```bash
PERFOLIZER_AGENT_CONFIG=/path/to/agent.json go run cmd/agent/main.go
```

The same config is used by UI client to connect to the agent.

### Remote process restart (via UI)

Perfolizer can restart a remote agent process from the UI by sending an admin command to the agent API.

1. Enable remote restart in the remote agent config:

```json
{
  "listen_host": "0.0.0.0",
  "port": 9090,
  "ui_poll_interval_seconds": 15,
  "enable_remote_restart": true,
  "remote_restart_token": "replace-with-strong-secret",
  "remote_restart_command": "sudo systemctl restart perfolizer-agent"
}
```

2. Start the agent with that config:

```bash
PERFOLIZER_AGENT_CONFIG=/path/to/agent.json go run cmd/agent/main.go
```

3. In UI open `Settings -> Agents`, select the agent and configure:
- `Restart token`: same value as `remote_restart_token`
- `Restart command`: optional; if empty, agent uses `remote_restart_command` from its config

4. Click `Restart process` in the agent runtime panel.

Notes:
- Remote restart endpoint is `POST /admin/restart`.
- If `enable_remote_restart` is `false`, restart returns `403`.
- If token is configured and does not match, restart returns `401`.
- Keep `remote_restart_token` secret and avoid exposing agent admin API publicly.

### Legacy GUI-only start

To start the graphical interface:

```bash
go run cmd/perfolizer/main.go
```

### Creating a Test Plan

1.  **Launch the App**: The main window will open with a default "Test Plan".
2.  **Tree Structure**:
    -   The left panel shows your Test Plan hierarchy.
    -   The right panel shows the properties of the selected element.
3.  **Edit Properties**:
    -   Select a node (e.g., "Thread Group 1") to change its settings (Number of Users, etc.).
    -   (Note: In this MVP version, adding new nodes via the UI context menu is the next planned feature. Currently, the tree structure is defined in `pkg/ui/app.go` setup).

## Project Structure

-   `cmd/perfolizer`: Entry point (Main application).
-   `cmd/agent`: Agent entry point (HTTP API for test execution + `/metrics`).
-   `pkg/core`: Core engine interfaces (`TestElement`, `Sampler`, `Context`).
-   `pkg/elements`: Implementation of components (Thread Groups, HttpSampler, Controllers).
-   `pkg/ui`: Fyne-based GUI implementation.
-   `pkg/agent`: Runtime execution agent and Prometheus exporter.
-   `pkg/config`: Shared configuration loader for agent/UI connectivity.
