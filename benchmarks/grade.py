"""Deterministic key-fact coverage: (covered + 0.5 * partial) / total."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Iterable


def wrap_prompt(question: str) -> str:
    """User text only. Do not mention Superopen or inject a pack."""
    return question.strip()


def wrap_code_prompt(question: str) -> str:
    """Same text for every compare arm. Forces a real working-tree session."""
    return (
        "Inspect this working tree before answering. Cite file paths and "
        "symbol names from the code you find. Do not rely on training data.\n\n"
        + question.strip()
    )


def wrap_swe_prompt(problem_statement: str) -> str:
    """Same issue text for native and Superopen SWE-bench arms. Do not mention Superopen."""
    return (
        "Fix this GitHub issue in the working tree. Do not commit. Leave the "
        "fix as unstaged or staged edits in this checkout.\n\n"
        + problem_statement.strip()
    )


def _haystack(text: str) -> str:
    return (text or "").lower()


def fact_hit(answer: str, aliases: list[str]) -> bool:
    blob = _haystack(answer)
    return any(alias.lower() in blob for alias in aliases if alias.strip())


def grade_answer(answer: str, key_facts: list[dict[str, Any]]) -> dict[str, Any]:
    total = len(key_facts)
    covered = 0
    partial = 0
    details: list[dict[str, Any]] = []
    for fact in key_facts:
        aliases = [str(a) for a in fact.get("aliases") or []]
        hit = fact_hit(answer, aliases)
        partial_aliases = [str(a) for a in fact.get("partial_aliases") or []]
        part = (not hit) and fact_hit(answer, partial_aliases)
        if hit:
            covered += 1
            state = "covered"
        elif part:
            partial += 1
            state = "partial"
        else:
            state = "miss"
        details.append(
            {
                "id": fact.get("id") or aliases[:1],
                "state": state,
                "aliases": aliases,
            }
        )
    if total == 0:
        coverage = 0.0
        verdict = "miss"
    else:
        coverage = (covered + 0.5 * partial) / total
        if covered == total:
            verdict = "covered"
        elif covered == 0 and partial == 0:
            verdict = "miss"
        else:
            verdict = "partial"
    return {
        "covered": covered,
        "partial": partial,
        "total": total,
        "coverage": coverage,
        "verdict": verdict,
        "facts": details,
    }


def load_questions(path: Path) -> list[dict[str, Any]]:
    doc = json.loads(path.read_text())
    questions = doc.get("questions") if isinstance(doc, dict) else doc
    if not isinstance(questions, list) or not questions:
        raise ValueError(f"no questions in {path}")
    return questions


def grade_gold_files(answer: str, gold_files: list[str]) -> dict[str, Any]:
    blob = _haystack(answer)
    paths = [str(p).strip() for p in gold_files if str(p).strip()]
    bases = [p.rsplit("/", 1)[-1] for p in paths]
    hits = []
    for path in paths:
        base = path.rsplit("/", 1)[-1]
        aliases = {path}
        if bases.count(base) == 1:
            aliases.add(base)
        if "/" in path:
            aliases.add("/".join(path.split("/")[-2:]))
        hit = any(alias.lower() in blob for alias in aliases if alias)
        hits.append({"path": path, "hit": hit})
    total = len(hits)
    covered = sum(1 for h in hits if h["hit"])
    return {
        "covered": covered,
        "total": total,
        "coverage": (covered / total) if total else 0.0,
        "files": hits,
    }


def graph_probe_grade(ok: bool, partial: bool = False) -> float:
    if ok:
        return 1.0
    if partial:
        return 0.5
    return 0.0


def recall_any_at_k(ranked: Iterable[Any], gold: Iterable[Any], k: int) -> bool:
    gold_set = {str(g) for g in gold if g is not None and str(g)}
    if not gold_set or k <= 0:
        return False
    top = [str(x) for x in list(ranked)[:k] if x is not None and str(x)]
    return any(item in gold_set for item in top)


def recall_payload(ranked: list[Any], gold: list[Any], ks: tuple[int, ...] = (5, 10)) -> dict[str, bool]:
    return {f"hit_at_{k}": recall_any_at_k(ranked, gold, k) for k in ks}
