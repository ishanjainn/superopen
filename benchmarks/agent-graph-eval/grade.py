"""Deterministic key-fact coverage and Claude JSON usage parsing."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path
from typing import Any

GRAPH_MARKERS = (
    "graph query",
    "graph_query",
    "graph_search",
    "graph_snippet",
    "graph_trace",
    "graph_architecture",
    "search_graph",
    "query_graph",
    "trace_path",
    "get_code_snippet",
    "get_architecture",
    "so graph",
    "knowledge graph",
    "code graph",
    "memory_recall",
)

MEMORY_MARKERS = (
    "so memory",
    "memory search",
    "memory get",
    "memory recall",
    "memory timeline",
    "memory_search",
    "memory_recall",
    "memory_get",
    "get_observations",
    "get_session",
    "mem-search",
)

MEMORY_INJECT_MARKERS = (
    "mem #",
    "so memory get",
    "prior work",
    "mem-search",
    "memory_recall",
    "failed to read dashboards config",
)

SHARED_INSTRUCTION = """{question}"""


def wrap_prompt(question: str) -> str:
    return SHARED_INSTRUCTION.format(question=question.strip())


def _haystack(text: str) -> str:
    return (text or "").lower()


def fact_hit(answer: str, aliases: list[str]) -> bool:
    blob = _haystack(answer)
    return any(alias.lower() in blob for alias in aliases if alias.strip())


def grade_answer(answer: str, key_facts: list[dict[str, Any]]) -> dict[str, Any]:
    total = len(key_facts)
    covered = 0
    details = []
    for fact in key_facts:
        aliases = [str(a) for a in fact.get("aliases") or []]
        hit = fact_hit(answer, aliases)
        if hit:
            covered += 1
        details.append({"id": fact.get("id") or aliases[:1], "covered": hit, "aliases": aliases})
    coverage = (covered / total) if total else 0.0
    if total == 0:
        verdict = "miss"
    elif covered == total:
        verdict = "covered"
    elif covered == 0:
        verdict = "miss"
    else:
        verdict = "partial"
    return {
        "covered": covered,
        "total": total,
        "coverage": coverage,
        "verdict": verdict,
        "facts": details,
    }


def worktree_diff(worktree: Path) -> str:
    proc = subprocess.run(
        ["git", "-C", str(worktree), "diff", "--", "."],
        capture_output=True,
        text=True,
        timeout=60,
    )
    return (proc.stdout or "") + (proc.stderr or "")


def grade_worktree(worktree: Path, spec: dict[str, Any]) -> dict[str, Any]:
    details = []
    ok = True
    for item in spec.get("expect_path_contains") or []:
        rel = str(item.get("path") or "")
        path = worktree / rel
        body = path.read_text(errors="replace") if path.is_file() else ""
        missing = [n for n in (item.get("needles") or []) if n not in body]
        hit = not missing and path.is_file()
        ok = ok and hit
        details.append({"path": rel, "ok": hit, "missing": missing})
    diff = worktree_diff(worktree)
    diff_files = spec.get("expect_diff_files") or []
    for rel in diff_files:
        hit = rel in diff or f"b/{rel}" in diff
        ok = ok and hit
        details.append({"diff_file": rel, "ok": hit})
    return {"ok": ok, "details": details}


def graph_used(text: str) -> bool:
    blob = _haystack(text)
    return any(marker in blob for marker in GRAPH_MARKERS)


def memory_used(text: str, transcript: dict[str, Any] | None = None) -> bool:
    if transcript:
        if int(transcript.get("memory_tools") or 0) > 0:
            return True
        if int(transcript.get("memory_injected") or 0) > 0:
            return True
    blob = _haystack(text)
    return any(marker in blob for marker in MEMORY_MARKERS)


def parse_usage(path: Path) -> dict[str, Any]:
    raw = path.read_text()
    data = json.loads(raw)
    usage = data.get("usage") or {}
    model_usage = data.get("modelUsage") or {}
    model = next(iter(model_usage.values()), {}) if model_usage else {}
    inp = model.get("inputTokens", usage.get("input_tokens", 0)) or 0
    cc = model.get("cacheCreationInputTokens", usage.get("cache_creation_input_tokens", 0)) or 0
    cr = model.get("cacheReadInputTokens", usage.get("cache_read_input_tokens", 0)) or 0
    out = model.get("outputTokens", usage.get("output_tokens", 0)) or 0
    result = data.get("result") or ""
    scan = raw if isinstance(raw, str) else result
    purity = {
        "mentions_graph": graph_used(scan),
        "mentions_read": "source read" in result.lower()
        or "files read" in result.lower()
        or "`src/" in result
        or "grep" in result.lower(),
    }
    return {
        "turns": data.get("num_turns"),
        "cost_usd": data.get("total_cost_usd"),
        "duration_ms": data.get("duration_ms"),
        "input_tokens": inp,
        "cache_create": cc,
        "cache_read": cr,
        "output_tokens": out,
        "input_side_total": inp + cc + cr,
        "result": result,
        "purity": purity,
        "is_error": data.get("is_error"),
        "session_id": data.get("session_id") or "",
    }


def filter_questions(questions: list[dict[str, Any]], ids: str) -> list[dict[str, Any]]:
    if not ids.strip():
        return questions
    want = [part.strip() for part in ids.split(",") if part.strip()]
    by_id = {q["id"]: q for q in questions}
    missing = [qid for qid in want if qid not in by_id]
    if missing:
        raise ValueError(f"unknown question ids: {missing}")
    return [by_id[qid] for qid in want]


def load_questions(path: Path) -> list[dict[str, Any]]:
    doc = json.loads(path.read_text())
    questions = doc.get("questions") if isinstance(doc, dict) else doc
    if not isinstance(questions, list) or not questions:
        raise ValueError(f"no questions in {path}")
    return questions
