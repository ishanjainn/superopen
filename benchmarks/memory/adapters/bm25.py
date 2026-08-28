"""Internal retrieval baselines over a shared text corpus."""

from __future__ import annotations

import math
import re
from collections import Counter
from typing import Any


def _tokenize(text: str) -> list[str]:
    return re.findall(r"[a-z0-9_]+", (text or "").lower())


class BM25Index:
    def __init__(self, docs: list[dict[str, Any]]) -> None:
        self.docs = docs
        self.df: Counter[str] = Counter()
        self.tf: list[Counter[str]] = []
        for doc in docs:
            tokens = _tokenize(doc.get("text", ""))
            c = Counter(tokens)
            self.tf.append(c)
            for t in set(tokens):
                self.df[t] += 1
        self.n = len(docs)
        self.avgdl = sum(sum(c.values()) for c in self.tf) / max(self.n, 1)

    def search(self, query: str, k: int = 10) -> list[str]:
        q = _tokenize(query)
        scores: list[tuple[float, str]] = []
        for i, doc in enumerate(self.docs):
            score = 0.0
            dl = sum(self.tf[i].values()) or 1
            for term in q:
                f = self.tf[i].get(term, 0)
                if f == 0:
                    continue
                idf = math.log(1 + (self.n - self.df[term] + 0.5) / (self.df[term] + 0.5))
                score += idf * f * 2.2 / (f + 1.2 * (0.25 + 0.75 * dl / self.avgdl))
            scores.append((score, doc["id"]))
        scores.sort(reverse=True)
        return [doc_id for _, doc_id in scores[:k]]


def dense_search(docs: list[dict[str, Any]], query: str, k: int = 10) -> list[str]:
    q = Counter(_tokenize(query))
    scores: list[tuple[float, str]] = []
    for doc in docs:
        d = Counter(_tokenize(doc.get("text", "")))
        dot = sum(q[t] * d.get(t, 0) for t in q)
        scores.append((float(dot), doc["id"]))
    scores.sort(reverse=True)
    return [doc_id for _, doc_id in scores[:k]]


def rrf_merge(lists: list[list[str]], k: int = 10, c: int = 60) -> list[str]:
    scores: dict[str, float] = {}
    for lst in lists:
        for rank, doc_id in enumerate(lst):
            scores[doc_id] = scores.get(doc_id, 0.0) + 1.0 / (c + rank + 1)
    ordered = sorted(scores.items(), key=lambda x: x[1], reverse=True)
    return [doc_id for doc_id, _ in ordered[:k]]
