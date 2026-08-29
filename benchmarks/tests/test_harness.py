#!/usr/bin/env python3
"""Offline harness tests — no API, no Django clone."""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

BENCH = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(BENCH))

from grade import grade_answer, graph_probe_grade  # noqa: E402
from memory.adapters.bm25 import BM25Index, bow_search, rrf_merge  # noqa: E402
from spend import SpendLedger  # noqa: E402


class HarnessTests(unittest.TestCase):
    def test_grade_partial_credit(self) -> None:
        g = grade_answer(
            "django ORM QuerySet lazy SQL",
            [
                {"id": "a", "aliases": ["QuerySet"]},
                {"id": "b", "aliases": ["middleware"], "partial_aliases": ["SQL"]},
            ],
        )
        self.assertEqual(g["covered"], 1)
        self.assertEqual(g["partial"], 1)
        self.assertAlmostEqual(g["coverage"], 0.75)

    def test_graph_probe_grades(self) -> None:
        self.assertEqual(graph_probe_grade(True), 1.0)
        self.assertEqual(graph_probe_grade(False, True), 0.5)
        self.assertEqual(graph_probe_grade(False), 0.0)

    def test_bm25_search(self) -> None:
        docs = [
            {"id": "1", "text": "django queryset lazy evaluation"},
            {"id": "2", "text": "middleware get_response chain"},
        ]
        idx = BM25Index(docs)
        hits = idx.search("queryset lazy", k=1)
        self.assertEqual(hits, ["1"])
        merged = rrf_merge([hits, bow_search(docs, "queryset lazy", k=1)], k=1)
        self.assertTrue(merged)

    def test_spend_ledger(self) -> None:
        ledger = SpendLedger(max_spend=1.0)
        ledger.record("a", 0.5)
        self.assertTrue(ledger.allow_llm())
        ledger.record("b", 0.4)
        ledger.check()
        with self.assertRaises(RuntimeError):
            ledger.record("c", 0.2)
            ledger.check()

    def test_wrap_prompt_is_user_text(self) -> None:
        from grade import wrap_code_prompt, wrap_prompt

        p = wrap_prompt("Where did we leave the JWT expiry?")
        self.assertEqual(p, "Where did we leave the JWT expiry?")
        self.assertNotIn("Superopen", p)
        self.assertNotIn("Memories:", p)

        c = wrap_code_prompt("How does Django middleware work?")
        self.assertIn("working tree", c.lower())
        self.assertIn("How does Django middleware work?", c)
        self.assertNotIn("Superopen", c)
        self.assertNotIn("graph query", c.lower())

    def test_scale_minimums(self) -> None:
        from argparse import Namespace

        from scale import apply_scale, default_model, require_coding_host, validate_sizes

        small = Namespace(scale="small", split="locomo", n=None, qa_n=None, compare_n=None, compare_ids="")
        apply_scale(small)
        validate_sizes(small)
        self.assertEqual(small.n, 100)
        self.assertEqual(small.compare_n, 6)

        full = Namespace(scale="full", split="locomo", n=None, qa_n=None, compare_n=None, compare_ids="")
        apply_scale(full)
        validate_sizes(full)
        self.assertEqual(full.n, 300)
        self.assertEqual(full.qa_n, 20)

        lme = Namespace(scale="small", split="longmemeval", n=None, qa_n=None, compare_n=None, compare_ids="")
        apply_scale(lme)
        validate_sizes(lme)
        self.assertEqual(lme.n, 50)

        toy = Namespace(scale="small", split="locomo", n=40, qa_n=40, compare_n=6, compare_ids="")
        with self.assertRaises(SystemExit):
            validate_sizes(toy)
        tiny_compare = Namespace(scale="small", split="locomo", n=100, qa_n=100, compare_n=3, compare_ids="")
        with self.assertRaises(SystemExit):
            validate_sizes(tiny_compare)
        self.assertEqual(require_coding_host("claude-code"), "claude-code")
        with self.assertRaises(SystemExit):
            require_coding_host("anthropic-api")
        self.assertEqual(default_model("claude-code", "opencode/big-pickle"), "claude-sonnet-5")

    def test_require_agent_credentials(self) -> None:
        import os
        from unittest.mock import patch

        from scale import require_agent_credentials

        require_agent_credentials(modes=["graph"], phase=3, max_spend=0.0)
        with patch.dict(os.environ, {"ANTHROPIC_API_KEY": ""}, clear=False):
            with self.assertRaises(SystemExit):
                require_agent_credentials(modes=["compare"], phase=2, max_spend=20.0)
        with patch.dict(os.environ, {"ANTHROPIC_API_KEY": "sk-test"}, clear=False):
            require_agent_credentials(modes=["compare"], phase=2, max_spend=20.0)
            with self.assertRaises(SystemExit):
                require_agent_credentials(modes=["memory"], phase=3, max_spend=0.0)
            require_agent_credentials(modes=["memory"], phase=3, max_spend=15.0)

    def test_load_repo_env(self) -> None:
        import os
        import tempfile
        from unittest.mock import patch

        from run import load_repo_env

        with tempfile.TemporaryDirectory() as tmp:
            env_file = Path(tmp) / ".env"
            env_file.write_text("ANTHROPIC_API_KEY=from-dotenv\n# comment\nFOO=bar\n")
            with patch.dict(os.environ, {}, clear=True):
                load_repo_env(Path(tmp))
                self.assertEqual(os.environ.get("ANTHROPIC_API_KEY"), "from-dotenv")
                self.assertEqual(os.environ.get("FOO"), "bar")

    def test_locomo_sample_is_stratified(self) -> None:
        from memory.runner import _qa_sample

        items = [{"id": f"{c}-{i}", "category": c} for c in ("1", "2", "3", "4", "5") for i in range(20)]
        out = _qa_sample(items, 10, "locomo")
        self.assertEqual(len(out), 10)
        cats = [str(x["category"]) for x in out]
        for c in ("1", "2", "3", "4", "5"):
            self.assertEqual(cats.count(c), 2, cats)

    def test_freeze_so_restores_diary_corpus(self) -> None:
        import tempfile

        from memory.runner import _freeze_so, _restore_so

        with tempfile.TemporaryDirectory() as tmp:
            store = Path(tmp) / "locomo"
            so = store / ".so"
            so.mkdir(parents=True)
            (so / "note").write_text("diary\n")
            frozen = _freeze_so(store)
            (so / "note").write_text("polluted\n")
            (so / "extra").write_text("question stub\n")
            _restore_so(store, frozen)
            self.assertEqual((store / ".so" / "note").read_text(), "diary\n")
            self.assertFalse((store / ".so" / "extra").exists())

    def test_recall_any_at_k_matches_published_definition(self) -> None:
        from grade import recall_any_at_k, recall_payload

        ranked = ["a", "b", "gold", "d"]
        gold = ["gold"]
        self.assertFalse(recall_any_at_k(ranked, gold, 2))
        self.assertTrue(recall_any_at_k(ranked, gold, 5))
        self.assertTrue(recall_any_at_k(ranked, gold, 10))
        self.assertFalse(recall_any_at_k(ranked, [], 10))
        hits = recall_payload(ranked, gold)
        self.assertTrue(hits["hit_at_5"])
        self.assertTrue(hits["hit_at_10"])

    def test_run_help_omits_debug(self) -> None:
        import subprocess

        proc = subprocess.run(
            [sys.executable, str(BENCH / "run.py"), "--help"],
            capture_output=True,
            text=True,
            cwd=str(BENCH.parent),
        )
        self.assertEqual(proc.returncode, 0)
        self.assertNotIn("--debug", proc.stdout)
        self.assertNotIn("debug.json", proc.stdout)

    def test_report_shape_shipped(self) -> None:
        root = BENCH.parent / "BENCHMARKS.md"
        text = root.read_text()
        self.assertIn("## System", text)
        self.assertIn("## Run", text)
        self.assertIn("| Score |", text)
        self.assertNotIn("Critical notice", text)

    def test_report_writes_full_markdown(self) -> None:
        import tempfile

        from report import render, write_benchmarks_md

        tmp = Path(tempfile.mkdtemp())
        summary = {
            "modes": ["memory", "graph", "compare"],
            "out": str(tmp),
            "scale": "small",
            "host": "claude-code",
            "model": "claude-sonnet-5",
            "n": 100,
            "isolate": "docker",
            "memory": {
                "split": "locomo",
                "n": 100,
                "adapters": {
                    "superopen": {
                        "recall_at_5": 0.85,
                        "recall_at_10": 0.9285714285714286,
                        "hits_at_5": 83,
                        "hits_at_10": 91,
                        "hits": 91,
                        "total": 98,
                    },
                    "bm25": {"recall_at_5": 0.88, "recall_at_10": 0.9081632653061225, "hits": 89, "total": 98},
                    "bow": {"recall_at_5": 0.3, "recall_at_10": 0.4, "hits": 40, "total": 98},
                    "rrf": {"recall_at_5": 0.8, "recall_at_10": 0.86, "hits": 85, "total": 98},
                },
                "qa_n": 20,
                "qa_llm": {
                    "superopen": {
                        "accuracy_judge": 1.0,
                        "hits_judge": 16,
                        "total": 16,
                        "skipped_no_gold": 4,
                        "host": "claude-code",
                    }
                },
            },
            "contradict": {"ok": True, "rescue_at_10": 1.0, "historical_verbatim": 1.0},
            "total_cost_usd": 0.57,
            "duration_sec": 1234,
            "mode_duration_sec": {"index": 164.2, "memory": 600, "compare": 470},
            "graph": {
                "index": {"ok": True, "elapsed_sec": 164.2, "nodes": 52355, "edges": 346672, "files": 4652},
                "graph": {"score": 12.0, "max": 12.0, "pct": 100.0, "hits": 12, "total": 12},
            },
            "compare": {
                "summary": {
                    "native_coverage_avg": 1.0,
                    "superopen_coverage_avg": 1.0,
                    "native_cost_usd": 0.95,
                    "superopen_cost_usd": 0.57,
                    "native_cache_read_tokens": 8_000_000,
                    "superopen_cache_read_tokens": 4_000_000,
                },
                "duration_sec": 900,
                "rows": [
                    {"arm": "native", "id": "orm", "cost_usd": 0.2},
                    {"arm": "superopen", "id": "orm", "cost_usd": 0.1},
                ],
            },
        }
        (tmp / "summary.json").write_text(json.dumps(summary))
        dest = write_benchmarks_md(tmp, dest=tmp / "BENCHMARKS.md")
        text = dest.read_text()
        self.assertIn("## System", text)
        self.assertIn("## Run", text)
        self.assertIn("Claude Code", text)
        self.assertIn("92.9%", text)
        self.assertIn("12/12", text)
        self.assertNotIn("Critical notice", text)
        self.assertIn("QA accuracy", text)
        self.assertIn("recall@5", text)
        self.assertIn("Rescue@10", text)
        self.assertIn("historical-verbatim", text)
        self.assertIn("index time", text)
        self.assertIn("| Duration |", text)
        self.assertIn("| Total cost |", text)
        self.assertIn("| Graph index |", text)
        self.assertIn("| Score |", text)
        self.assertNotIn("| Internal |", text)
        self.assertIn("$0.57", text)
        self.assertIn("20m 34s", text)
        self.assertIn("2m 44s", text)
        self.assertNotIn("Superopen fully beats", text)
        pending = render({"scale": "small", "host": "claude-code", "isolate": "docker"})
        self.assertIn("pending", pending)
        self.assertNotIn("not in this stamp", pending)
        self.assertIn("## System", pending)
        self.assertNotIn("Critical notice", pending)

    def test_report_fill_from_missing_suites(self) -> None:
        import tempfile

        from report import write_benchmarks_md

        primary = Path(tempfile.mkdtemp())
        extra = Path(tempfile.mkdtemp())
        (primary / "summary.json").write_text(
            json.dumps(
                {
                    "scale": "small",
                    "isolate": "docker",
                    "compare": {
                        "summary": {
                            "native_coverage_avg": 1.0,
                            "superopen_coverage_avg": 1.0,
                            "native_cost_usd": 1.0,
                            "superopen_cost_usd": 0.6,
                        }
                    },
                }
            )
        )
        (extra / "summary.json").write_text(
            json.dumps(
                {
                    "memory": {
                        "split": "longmemeval",
                        "n": 50,
                        "adapters": {
                            "superopen": {"recall_at_5": 0.72, "recall_at_10": 0.78, "hits": 39, "total": 50},
                            "bm25": {"recall_at_10": 0.76, "hits": 38, "total": 50},
                        },
                    }
                }
            )
        )
        text = write_benchmarks_md(primary, fill_from=[extra], dest=primary / "BENCHMARKS.md").read_text()
        self.assertIn("78.0%", text)
        self.assertIn("1.00", text)

    def test_dataset_fetch_rejects_non_https(self) -> None:
        import tempfile

        from memory.fetch_datasets import fetch_https

        dest = Path(tempfile.mkdtemp()) / "locomo10.json"
        with self.assertRaises(ValueError):
            fetch_https("http://example.invalid/locomo10.json", dest)
        with self.assertRaises(ValueError):
            fetch_https("javascript:alert(1)", dest)

    def test_django_questions_json(self) -> None:
        path = BENCH / "questions" / "django.json"
        data = json.loads(path.read_text())
        self.assertEqual(len(data["questions"]), 6)

    def test_dataset_readme_layout(self) -> None:
        from memory.fetch_datasets import ensure_readme, dataset_path

        ensure_readme(BENCH / "datasets")
        self.assertEqual(dataset_path("locomo").name, "locomo10.json")

    def test_docker_run_script_is_isolated(self) -> None:
        script = (BENCH / "docker-run.sh").read_text()
        self.assertTrue((BENCH / "docker-run.sh").is_file())
        self.assertIn("--isolate host", script)
        self.assertIn("/usr/local/bin/so:ro", script)
        self.assertNotIn('"$HOME:', script)
        self.assertNotIn("$HOME:", script)
        self.assertIn("Auth files only", script)

    def test_docker_never_mounts_developer_home(self) -> None:
        import tempfile

        import isolate
        from docker import (
            assert_isolated,
            forbidden_volume_source,
            is_linux_elf,
            prepare_guest_so,
            rewrite_cmd,
            start_arm_argv,
        )

        home = Path.home()
        self.assertEqual(forbidden_volume_source(str(home), home), str(home.resolve()))
        self.assertIsNotNone(forbidden_volume_source(str(home / ".claude"), home))
        self.assertIsNone(forbidden_volume_source("/tmp/so-bench-arm-home", home))
        with self.assertRaises(RuntimeError):
            assert_isolated(["docker", "run", "-v", f"{home}:/eval/home"], home=home)

        work = Path("/tmp/so-bench-work")
        arm_home = Path("/tmp/so-bench-arm-home")
        claude = Path("/tmp/so-bench-arm-claude")
        so_linux = Path("/tmp/so-linux")
        argv = start_arm_argv(worktree=work, home=arm_home, claude=claude, so_linux=so_linux)
        blob = " ".join(argv)
        self.assertNotIn(str(home), blob)
        self.assertTrue(any("/usr/local/bin/so:ro" in a for a in argv), argv)
        self.assertTrue(any("so-linux" in a for a in argv), argv)

        work_r = str(work.resolve())
        rewritten = rewrite_cmd(
            [str(so_linux), "init", "--root", str(work)],
            [(work_r, "/work")],
            str(so_linux),
        )
        self.assertEqual(rewritten[0], "/usr/local/bin/so")
        self.assertEqual(rewritten[-1], "/work")

        with tempfile.NamedTemporaryFile(delete=False) as handle:
            handle.write(b"\x7fELF" + b"\0" * 32)
            elf = Path(handle.name)
        self.assertTrue(is_linux_elf(elf))
        self.assertEqual(prepare_guest_so(str(elf), BENCH.parent), elf.resolve())
        elf.unlink()

        self.assertEqual(isolate.mode(), isolate.ISOLATE_HOST)

    def test_debug_vs_bm25_gate(self) -> None:
        from debug import _vs_bm25

        so = {"recall_at_10": 0.93, "hits": 91, "total": 98}
        bm = {"recall_at_10": 0.91, "hits": 89, "total": 98}
        line = _vs_bm25("LOCOMO", so, bm)
        self.assertIn("beats BM25", line)
        tied = _vs_bm25("LongMemEval-S", {"recall_at_10": 0.76, "hits": 38, "total": 50}, {"recall_at_10": 0.76, "hits": 38, "total": 50}, target=0.90)
        self.assertIn("ties BM25", tied)
        self.assertIn("below 90%", tied)

    def test_debug_compare_cache_read_is_the_cost_hole(self) -> None:
        import tempfile
        from debug import _debug_compare

        tmp = Path(tempfile.mkdtemp())
        (tmp / "compare.json").write_text(
            json.dumps(
                {
                    "rows": [
                        {"arm": "native", "id": "orm", "coverage": 1.0, "input_tokens": 900, "cache_read_tokens": 0, "output_tokens": 12, "cost_usd": 0.1, "ok": True, "tools_likely": False},
                        {"arm": "superopen", "id": "orm", "coverage": 1.0, "input_tokens": 20, "cache_read_tokens": 800000, "cache_creation_tokens": 20000, "output_tokens": 4000, "cost_usd": 0.3, "ok": True, "tools_likely": True},
                    ],
                    "summary": {
                        "native_coverage_avg": 1.0,
                        "superopen_coverage_avg": 1.0,
                        "native_uncached_input": 900,
                        "superopen_uncached_input": 20,
                        "native_cache_read_tokens": 0,
                        "superopen_cache_read_tokens": 800000,
                        "native_output_tokens": 12,
                        "superopen_output_tokens": 4000,
                        "native_cost_usd": 0.1,
                        "superopen_cost_usd": 0.3,
                    },
                }
            )
        )
        got = " ".join(_debug_compare(tmp)["verdict"])
        self.assertIn("coverage Superopen", got)
        self.assertIn("cache_read", got)
        self.assertIn("USD Superopen", got)

    def test_judge_parse_and_cache_key(self) -> None:
        from judge import cache_key, parse_verdict

        self.assertTrue(parse_verdict('{"hit": true}'))
        self.assertFalse(parse_verdict('{"hit": false}'))
        self.assertTrue(parse_verdict("```json\n{\"hit\": true}\n```"))
        self.assertIsNone(parse_verdict("not json"))
        a = cache_key("conv-26-1", "May 7, 2023")
        b = cache_key("conv-26-1", "7 May 2023")
        self.assertNotEqual(a, b)
        self.assertTrue(a.startswith("conv-26-1-"))

    def test_claude_jsonl_usage_delta_and_so_invoked(self) -> None:
        import tempfile

        from hosts.claude_code import jsonl_sizes, usage_from_new_jsonl

        tmp = Path(tempfile.mkdtemp())
        projects = tmp / "projects" / "work"
        projects.mkdir(parents=True)
        session = projects / "session.jsonl"
        first = {
            "type": "assistant",
            "message": {
                "usage": {"input_tokens": 10, "output_tokens": 4, "cache_read_input_tokens": 100},
                "content": [{"type": "text", "text": "hi"}],
            },
        }
        session.write_text(json.dumps(first) + "\n")
        before = jsonl_sizes(tmp)
        second = {
            "type": "assistant",
            "message": {
                "usage": {"input_tokens": 20, "output_tokens": 30000, "cache_read_input_tokens": 4000000},
                "content": [
                    {
                        "type": "tool_use",
                        "name": "Bash",
                        "input": {"command": "/usr/local/bin/so memory recall 'who is Caroline'"},
                    }
                ],
            },
        }
        with session.open("a") as fh:
            fh.write(json.dumps(second) + "\n")
        got = usage_from_new_jsonl(tmp, before)
        self.assertGreaterEqual(got["cache_read_tokens"], 4_000_000)
        self.assertGreaterEqual(got["output_tokens"], 30_000)
        self.assertTrue(got["so_invoked"])


if __name__ == "__main__":
    unittest.main()
