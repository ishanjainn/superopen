# Model-neutral paired lifecycle release gate

This is an opt-in paid evaluation, not SWE-bench and not Graphify's ERPNext key-fact benchmark. Run control and treatment in fresh sessions pinned to one coding-agent version and model. Claude Haiku is one supported low-cost test selection, not product behavior; release evidence also includes a supported non-Claude profile.

The full raw ledger contains exactly 16 pairs: eight `question` tasks and eight `patch` tasks. A schema-v2 `compact` profile contains 10 pairs and must include one question and one patch for every task class: `local_lookup`, `cross_cutting`, `impact_analysis`, `temporal_update`, and `semantic_document`. Each pair records repository/commit identity, randomized arm order, prompts and budgets, success/gold coverage or deterministic test results, model/cache tokens, dollars, refresh cost, turns, tool calls, files read, graph calls, truncation, final-response completion, and post-edit graph state. Keep initialization cost per repository separate.

Control uses a clean checkout with no `.so`. Treatment follows the developer workflow exactly: `so install`, a fresh coding-agent `/so init` that reaches a verified semantic graph, then a fresh session for every task. Do not reuse task conversation context between arms or tasks.

After capturing the raw JSON ledger, evaluate it with:

```bash
so --json graph benchmark --release-ledger /absolute/path/to/agent-ledger.json
```

The command rejects a ledger unless the model is identified and the 8/8 task shape is exact. Schema v2 blocks release unless treatment success is non-inferior, graph adoption is at least 80% on genuinely graph-suitable tasks, local-task median overhead stays within 5%, graph-suitable truncation stays within 10%, and cumulative cost per successful result breaks even by 25 sessions. No treatment may lose a final response when control completes, and every treatment patch must end `ready` or `continuation_required`. Results report raw task cost, initialization, refresh, cost per successful result, 1/10/25/50-session projections, and measured break-even separately.

Required top-level fields:

```json
{
  "schema_version": 2,
  "profile": "compact",
  "model": "provider-model-id",
  "agent_version": "pinned-version",
  "initialization_cost_usd": {"repo-id": 0.0},
  "pairs": []
}
```

Each pair uses `id`, `repository`, `kind`, `task_class`, `graph_suitable`, `control`, and `treatment`; repeated-work cohorts may add `repeat_group` and `repeat_index`. Each run minimally includes `success`, `cost_usd`, `graph_calls`, and `final_response`; it may add `refresh_cost_usd`, token classes, turns, and `truncated`. Count both explicit agent graph commands and successful automatic-orientation `graph_orientation` audit events in `graph_calls`. Patch treatments also include `post_edit_graph_state`. Preserve richer raw measurements alongside these evaluator fields rather than replacing them with aggregates. Schema-v1 ledgers remain readable and retain the original 10%-after-16-tasks rule for historical comparisons.
