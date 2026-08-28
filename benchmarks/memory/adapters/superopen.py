"""Superopen memory adapter (so memory capture / recall / get)."""

from __future__ import annotations

import json
import os
import re
from pathlib import Path
from typing import Any

import isolate


def _run(
    cmd: list[str],
    root: Path,
    env: dict[str, str] | None = None,
    timeout: int = 300,
):
    merged = env if env is not None else os.environ.copy()
    return isolate.run(cmd, cwd=root, env=merged, timeout=timeout)


def capture_episode(
    so_bin: str,
    root: Path,
    title: str,
    text: str,
    session: str = "",
    env: dict[str, str] | None = None,
) -> dict[str, Any]:
    root = root.resolve()
    cmd = [
        so_bin,
        "--json",
        "memory",
        "capture",
        "--kind",
        "session",
        "--horizon",
        "medium",
        "--title",
        title,
        "--text",
        text,
        "--root",
        str(root),
    ]
    if session:
        cmd.extend(["--session", session])
    proc = _run(cmd, root, env=env, timeout=300)
    stdout = proc.stdout or ""
    got_id = None
    got_title = title
    try:
        payload = json.loads(stdout or "{}")
        data = payload.get("data") or payload
        if isinstance(data, dict) and data.get("id") is not None:
            got_id = int(data["id"])
            if data.get("title"):
                got_title = str(data["title"])
    except (json.JSONDecodeError, TypeError, ValueError):
        pass
    if got_id is None:
        m = re.search(r"captured #(\d+)\s+(.*)", stdout.strip().splitlines()[-1] if stdout.strip() else "")
        got_id = int(m.group(1)) if m else None
        got_title = m.group(2).strip() if m else got_title
    collapsed = bool(got_title and got_title != title)
    skipped = proc.returncode != 0 or "capture skipped" in (proc.stderr or "") + stdout
    return {
        "ok": proc.returncode == 0,
        "id": got_id,
        "title": got_title or title,
        "requested": title,
        "collapsed": collapsed,
        "skipped": skipped,
        "stderr": (proc.stderr or "")[-400:],
    }


def search_hits(
    so_bin: str,
    root: Path,
    query: str,
    limit: int = 10,
    env: dict[str, str] | None = None,
) -> list[dict[str, Any]]:
    root = root.resolve()
    proc = _run(
        [so_bin, "--json", "memory", "search", query, "--limit", str(limit), "--root", str(root)],
        root,
        env=env,
    )
    if proc.returncode != 0:
        return []
    try:
        payload = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError:
        return []
    items = payload.get("items") or payload.get("data") or []
    if isinstance(items, dict):
        items = items.get("items") or []
    out: list[dict[str, Any]] = []
    for it in items:
        if not isinstance(it, dict):
            continue
        out.append(
            {
                "id": it.get("id"),
                "title": str(it.get("title") or ""),
                "kind": it.get("kind"),
                "tokens": it.get("tokens"),
            }
        )
    return out


def search(so_bin: str, root: Path, query: str, limit: int = 10, env: dict[str, str] | None = None) -> list[str]:
    return [h["title"] for h in search_hits(so_bin, root, query, limit, env=env) if h.get("title")]


def recall_hits(
    so_bin: str,
    root: Path,
    query: str,
    limit: int = 10,
    env: dict[str, str] | None = None,
) -> list[dict[str, Any]]:
    root = root.resolve()
    proc = _run(
        [so_bin, "--json", "memory", "recall", query, "--root", str(root)],
        root,
        env=env,
        timeout=300,
    )
    if proc.returncode != 0:
        return []
    try:
        payload = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError:
        return []
    data = payload.get("data") or payload
    hits = []
    if isinstance(data, dict):
        hits = data.get("hits") or data.get("items") or []
    elif isinstance(data, list):
        hits = data
    out: list[dict[str, Any]] = []
    for it in hits[:limit]:
        if not isinstance(it, dict):
            continue
        out.append(
            {
                "id": it.get("id"),
                "title": str(it.get("title") or ""),
                "kind": it.get("kind"),
                "tokens": it.get("tokens"),
                "text": str(it.get("text") or ""),
            }
        )
    return out


def rank_doc_ids(
    so_bin: str,
    root: Path,
    query: str,
    id_to_docs: dict[int, Any],
    limit: int = 10,
    env: dict[str, str] | None = None,
) -> list[str]:
    ranked: list[str] = []
    seen: set[str] = set()
    for h in recall_hits(so_bin, root, query, limit=limit, env=env):
        eid = h.get("id")
        if eid is None:
            continue
        try:
            key = int(eid)
        except (TypeError, ValueError):
            continue
        mapped = id_to_docs.get(key)
        if mapped is None:
            continue
        if isinstance(mapped, str):
            mapped = [mapped]
        for doc in mapped:
            if not doc or doc in seen:
                continue
            seen.add(doc)
            ranked.append(doc)
    return ranked


def recall_pack_text(
    so_bin: str, root: Path, query: str, limit: int = 10, env: dict[str, str] | None = None
) -> str:
    hits = recall_hits(so_bin, root, query, limit=limit, env=env)
    titled = []
    for h in hits:
        title = str(h.get("title") or "")
        body = str(h.get("text") or "")
        hid = h.get("id")
        titled.append(f"#{hid} {title}\n{body}".strip())
    return "\n\n".join(titled)


def embedder_id(so_bin: str, root: Path, env: dict[str, str] | None = None) -> str:
    root = root.resolve()
    proc = _run(
        [so_bin, "--json", "memory", "status", "--root", str(root)],
        root,
        env=env,
        timeout=30,
    )
    if proc.returncode != 0:
        return ""
    try:
        payload = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError:
        return ""
    data = payload.get("data") or payload
    if isinstance(data, dict):
        return str(data.get("embedder_id") or "")
    return ""


def get_texts(so_bin: str, root: Path, ids: list[Any], env: dict[str, str] | None = None) -> list[str]:
    want = [str(i) for i in ids if i is not None]
    if not want:
        return []
    root = root.resolve()
    proc = _run(
        [so_bin, "--json", "memory", "get", *want, "--root", str(root)],
        root,
        env=env,
    )
    if proc.returncode != 0:
        return []
    try:
        payload = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError:
        return []
    data = payload.get("data") or payload.get("items") or payload
    rows = data if isinstance(data, list) else [data]
    texts: list[str] = []
    for row in rows:
        if isinstance(row, dict):
            texts.append(str(row.get("text") or ""))
    return texts


def stored_titles(so_bin: str, root: Path, env: dict[str, str] | None = None) -> list[str]:
    hits = search_hits(so_bin, root, "", limit=200, env=env)
    return [h["title"] for h in hits if h.get("title")]
