# pkg

This directory contains the product code split by responsibility.

## Package Map

- [agent/README.md](agent/README.md): execution agent, HTTP surfaces, Prometheus metrics, restart/admin flow.
- [config/README.md](config/README.md): shared config loading and derived UI/agent addresses.
- [core/README.md](core/README.md): project model, persistence, runtime context, interfaces, stats.
- [elements/README.md](elements/README.md): thread groups, controllers, samplers, and factory registration.
- [ui/README.md](ui/README.md): desktop UI, editor flows, settings, dashboard, and agent client.

## How The Pieces Fit

1. The UI builds or loads a project and edits element properties.
2. The active test plan is serialized through `pkg/core`.
3. The plan is sent to the agent over HTTP.
4. The agent executes element trees from `pkg/elements`.
5. Runtime metrics are aggregated through `pkg/core.StatsRunner`.
6. The UI polls `/metrics` and updates dashboard/settings views.

## Extension Rule Of Thumb

- If you are adding behavior to the load model, start in `core` and `elements`.
- If you are adding operator workflow, start in `ui`.
- If you are changing runtime or observability surfaces, start in `agent`.
- If you are changing startup or executable layout, start in `cmd`.
