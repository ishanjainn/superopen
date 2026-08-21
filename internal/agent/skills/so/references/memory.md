# Superopen memory (prior work)

Load this only when the user asks about prior decisions in **this** repo. Skip on a cold clone (no session history).

Memory is hints, not authority. Graph answers “where is X”; memory answers “what did we decide.” Do not dump `.so/sessions/*/events.jsonl`.

| Intent | CLI |
|--------|-----|
| Find a prior decision | `__SO_BIN__ memory search "..."` |
| Read one hit | `__SO_BIN__ memory get <id>` |
| Save a rollup (at most once) | `__SO_BIN__ memory capture --request … --learned … --next …` |

Fetch bodies by the `id` field. Distill at most once when asked; skip if unrelated.
