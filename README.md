# Perfolizer

Perfolizer is a lightweight, JMeter-like load testing tool written in Golang. It provides a native GUI for creating and running performance tests using a tree-based structure, similar to JMeter, but powered by the efficiency of Go.

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

## Prerequisites

-   **Go 1.20+**: Ensure Go is installed and available in your PATH.
-   **Fyne Dependencies**: You may need C compilers for Fyne (usually `gcc` on Linux/Mac, or Mingw on Windows) if not already present, though pure Go modules often suffice for basic builds.

## Installation & Setup

1.  **Clone the repository**:
    ```bash
    git clone https://github.com/yourusername/perfolizer.git
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
-   `pkg/core`: Core engine interfaces (`TestElement`, `Sampler`, `Context`).
-   `pkg/elements`: Implementation of components (Thread Groups, HttpSampler, Controllers).
-   `pkg/ui`: Fyne-based GUI implementation.
