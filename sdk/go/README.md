# Coding-agent telemetry helpers

Internal OpenTelemetry span bootstrap used by `so coding hook` to persist
coding-agent session events locally. It contains no network exporter. **Not** a
general OpenAI/Anthropic/vLLM instrumentation SDK.

## Layout

| Path | Role |
|---|---|
| `sdk.go` / `config.go` | `Init` / `Shutdown` for the short-lived hook process |
| `semconv/` | Coding-agent + gen_ai attribute names |
| `helpers/` | Small shared helpers (e.g. capture flags) |

Consumed by `internal/codingotlp` and the vendor hook adapters. Do not add
provider-specific LLM client wrappers here.
