# pkg/ai

This package owns optional AI-assisted authoring flows for Perfolizer.

## Responsibilities

- AI settings defaults and provider selection.
- Rules-first draft generation for common performance-testing intents.
- Provider abstraction for remote or local LLM backends.
- Draft and patch validation against the existing `pkg/core` persistence model.
- Selection explanation helpers for the UI.

## Current Scope

- Draft a test plan from URL, request stats, and free-form intent.
- Suggest simple plan patches for thread-group shape changes.
- Explain the currently selected node and recommend a better thread-group style when the intent clearly implies one.
- Support OpenAI-compatible HTTP providers and a Codex CLI-backed provider with interactive browser login.
- Keep OpenAI API keys in OS-backed secure storage instead of normal app preferences.

## Important Constraints

- AI remains optional; the editor must still be fully usable without it.
- Drafts and patches are validated through `pkg/core` before the UI can apply them.
- The rules layer handles obvious cases first so simple requests do not require a remote model call.
