# pkg/elements

This package contains concrete load-testing elements used inside a test plan tree.

## Current Element Types

### Thread groups

- `SimpleThreadGroup`
- `RPSThreadGroup`

### Samplers

- `HttpSampler`

### Controllers

- `LoopController`
- `IfController`
- `PauseController`

## Key Files

- `threadgroups.go`: concurrent execution strategies and parameter injection into worker contexts.
- `samplers.go`: HTTP sampler execution, rate limiting, parameter extraction.
- `controllers.go`: flow-control elements.
- `json_helper.go`: simple JSON-path extraction used by HTTP sampler parameter extraction.

## Element Authoring Rules

When adding a new element type:

1. Register a factory in `init()`.
2. Implement `GetType()` and `GetProps()` if the element must round-trip through persistence.
3. Implement `Clone()` correctly.
4. Add UI support in `pkg/ui/app.go`.
5. Add persistence or behavior coverage in `tests/`.

## Runtime Notes

- Thread groups are usually the top-level executable children of the plan root.
- Thread groups own the shared HTTP runtime settings used by descendant samplers, including request timeout and keep-alive policy.
- `RPSThreadGroup` uses shared limiter state and profile blocks.
- `HttpSampler` currently owns variable extraction from response bodies via regexp or simple JSON path evaluation.
- `IfController` is currently serialized without a scriptable condition payload, so its JSON persistence is intentionally minimal.
