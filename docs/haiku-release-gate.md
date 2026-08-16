# Paired coding-agent release gate

This is an opt-in paid evaluation, not SWE-bench and not Graphify's ERPNext key-fact benchmark. Run control and treatment in fresh sessions pinned to one coding-agent version and model. Claude Haiku is one supported test selection, not product behavior.

The raw ledger must contain exactly 16 pairs: eight `question` tasks and eight `patch` tasks. Each pair records repository/commit identity, randomized arm order, prompts and budgets, success/gold coverage or deterministic test results, model/cache tokens, dollars, turns, tool calls, files read, graph calls, truncation, final-response completion, and post-edit graph state. Keep initialization cost per repository separate; the evaluator amortizes it across that repository's treatment tasks.

Control uses a clean checkout with no `.so`. Treatment follows the developer workflow exactly: `so install`, a fresh coding-agent `/so init` that reaches a verified semantic graph, then a fresh session for every task. Do not reuse task conversation context between arms or tasks.

After capturing the raw JSON ledger, evaluate it with:

```bash
so --json graph benchmark --release-ledger /absolute/path/to/haiku-ledger.json
```

The command rejects a ledger unless the model is identified and the 8/8 task shape is exact. It blocks release unless treatment success is non-inferior, amortized median cost is at least 10% lower, graph adoption is at least 80% on graph-suitable tasks, no treatment loses a final response when control completes, and every treatment patch ends `ready` or `continuation_required`.

Required top-level fields:

```json
{
  "schema_version": 1,
  "model": "claude-haiku-4-5-20251001",
  "agent_version": "pinned-version",
  "initialization_cost_usd": {"repo-id": 0.0},
  "pairs": []
}
```

Each pair uses `id`, `repository`, `kind`, `graph_suitable`, `control`, and `treatment`. Each run minimally includes `success`, `cost_usd`, `graph_calls`, and `final_response`; patch treatments also include `post_edit_graph_state`. Preserve richer raw measurements alongside these evaluator fields rather than replacing them with aggregates.
