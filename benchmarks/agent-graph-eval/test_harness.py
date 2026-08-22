#!/usr/bin/env python3
"""Harness unit tests. No live Claude, no grafana clone."""

from __future__ import annotations

import json
import os
import tempfile
import unittest
from pathlib import Path

import grade
import isolate
import run_eval


class ArmEnvTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.work = Path(self.tmp.name)
        self.paths = isolate.arm_paths(self.work, "vanilla")
        isolate.ensure_dirs(self.paths)

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def test_required_keys(self) -> None:
        env = isolate.arm_env(self.paths)
        for key in (
            "HOME",
            "CLAUDE_CONFIG_DIR",
            "XDG_CONFIG_HOME",
            "XDG_CACHE_HOME",
            "XDG_DATA_HOME",
            "SUPEROPEN_INSTALL_DIR",
        ):
            self.assertIn(key, env)
            self.assertTrue(env[key], msg=key)
        self.assertEqual(env["HOME"], str(self.paths["home"]))
        self.assertEqual(env["CLAUDE_CONFIG_DIR"], str(self.paths["claude"]))
        self.assertNotIn("SUPEROPEN_HOOK_STRICT", env)

    def test_home_is_not_developer_home(self) -> None:
        env = isolate.arm_env(self.paths)
        self.assertNotEqual(env["HOME"], str(Path.home()))
        self.assertNotEqual(env["CLAUDE_CONFIG_DIR"], str(Path.home() / ".claude"))

    def test_empty_settings(self) -> None:
        settings = json.loads((self.paths["claude"] / "settings.json").read_text())
        self.assertEqual(settings, {})


class AuthCopyTest(unittest.TestCase):
    def test_copies_credentials_only(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            src = Path(raw) / "src"
            dest = Path(raw) / "dest"
            src.mkdir()
            (src / ".credentials.json").write_text('{"ok": true}\n')
            (src / "settings.json").write_text('{"servers": {"leak": {}}}\n')
            (src / "hooks.json").write_text("{}\n")
            copied = isolate.copy_claude_auth(dest, src_claude=src)
            self.assertEqual(copied, [".credentials.json"])
            self.assertTrue((dest / ".credentials.json").is_file())
            self.assertFalse((dest / "settings.json").exists())
            self.assertFalse((dest / "hooks.json").exists())


class GraderTest(unittest.TestCase):
    def test_full_coverage(self) -> None:
        facts = [{"id": "a", "aliases": ["DashboardProvisioner"]}, {"id": "b", "aliases": ["yaml"]}]
        g = grade.grade_answer("The DashboardProvisioner reads YAML files.", facts)
        self.assertEqual(g["covered"], 2)
        self.assertEqual(g["coverage"], 1.0)
        self.assertEqual(g["verdict"], "covered")

    def test_zero_coverage(self) -> None:
        facts = [{"id": "a", "aliases": ["DashboardProvisioner"]}]
        g = grade.grade_answer("No idea.", facts)
        self.assertEqual(g["coverage"], 0.0)
        self.assertEqual(g["verdict"], "miss")

    def test_partial_coverage(self) -> None:
        facts = [
            {"id": "a", "aliases": ["ngalert"]},
            {"id": "b", "aliases": ["Alertmanager"]},
        ]
        g = grade.grade_answer("ngalert evaluates rules.", facts)
        self.assertEqual(g["covered"], 1)
        self.assertEqual(g["verdict"], "partial")
        self.assertAlmostEqual(g["coverage"], 0.5)

    def test_wrap_prompt_identical(self) -> None:
        q = "How does ngalert evaluate rules?"
        a = grade.wrap_prompt(q)
        b = grade.wrap_prompt(q)
        self.assertEqual(a, b)
        self.assertIn(q, a)
        self.assertEqual(a, q)

    def test_parse_usage_input_side(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "out.json"
            path.write_text(
                json.dumps(
                    {
                        "num_turns": 3,
                        "total_cost_usd": 0.12,
                        "duration_ms": 1000,
                        "usage": {
                            "input_tokens": 10,
                            "cache_creation_input_tokens": 20,
                            "cache_read_input_tokens": 30,
                            "output_tokens": 4,
                        },
                        "result": "used graph_query then Read",
                    }
                )
            )
            u = grade.parse_usage(path)
            self.assertEqual(u["turns"], 3)
            self.assertEqual(u["input_side_total"], 60)
            self.assertEqual(u["output_tokens"], 4)
            self.assertTrue(u["purity"]["mentions_graph"])


class HarnessHelpersTest(unittest.TestCase):
    def test_parse_nodes_edges(self) -> None:
        self.assertEqual(
            isolate.parse_nodes_edges("Initialized native graph: 12 nodes, 34 edges"),
            (12, 34),
        )
        self.assertEqual(isolate.parse_nodes_edges("Rebuilt: 9 nodes 2 edges"), (9, 2))

    def test_parse_rss_bytes(self) -> None:
        blob = "      334.88 real\n          6500958208  maximum resident set size\n"
        self.assertEqual(isolate.parse_rss_bytes(blob), 6500958208)
        self.assertIsNone(isolate.parse_rss_bytes("no rss"))

    def test_normalize_arm_aliases(self) -> None:
        self.assertEqual(run_eval.normalize_arm("so"), "superopen")
        self.assertEqual(run_eval.normalize_arm("vanilla"), "vanilla")

    def test_require_bins(self) -> None:
        msg = run_eval.require_bins(["superopen"], {"so": None})
        self.assertIsNotNone(msg)
        self.assertIn("--so-bin", msg or "")
        self.assertIsNone(run_eval.require_bins(["vanilla"], {"so": None}))

    def test_questions_load(self) -> None:
        qs = grade.load_questions(Path(__file__).resolve().parent / "questions" / "grafana.json")
        self.assertEqual(len(qs), 6)
        ids = {q["id"] for q in qs}
        self.assertIn("provisioning-dashboards", ids)
        self.assertIn("grafana-live", ids)
        for q in qs:
            self.assertTrue(q["prompt"])
            self.assertGreaterEqual(len(q["key_facts"]), 3)

    def test_linux_questions_load(self) -> None:
        qs = grade.load_questions(Path(__file__).resolve().parent / "questions" / "linux.json")
        self.assertEqual(len(qs), 6)
        ids = {q["id"] for q in qs}
        self.assertIn("cfs-pick-next", ids)
        self.assertIn("module-load", ids)
        self.assertEqual(run_eval.default_questions("linux").name, "linux.json")
        self.assertIn("linux", isolate.KNOWN_REPOS)

    def test_superopen_product_hooks(self) -> None:
        manifest = Path(__file__).resolve().parent.parent.parent / "plugins" / "claude-code" / "hooks" / "hooks.json"
        hooks = json.loads(manifest.read_text())["hooks"]
        self.assertIn("SessionStart", hooks)
        self.assertIn("PreToolUse", hooks)
        self.assertIn("PostToolUse", hooks)
        self.assertIn("SubagentStart", hooks)
        matchers = {entry.get("matcher") for entry in hooks["PreToolUse"]}
        self.assertIn("Bash|Grep", matchers)
        self.assertIn("Read|Glob", matchers)


class AggregateTest(unittest.TestCase):
    def test_aggregate_sums_cost_and_tokens(self) -> None:
        questions = [{"id": "a"}, {"id": "b"}]
        per_q = {
            "a": {
                "cost_usd": 0.1,
                "input_side_total": 100,
                "turns": 2,
                "coverage": {"coverage": 1.0},
                "purity": {"mentions_graph": True},
            },
            "b": {
                "cost_usd": 0.2,
                "input_side_total": 50,
                "turns": 4,
                "coverage": {"coverage": 0.5},
                "purity": {"mentions_graph": False},
            },
        }
        agg = run_eval.aggregate_arm(questions, per_q)
        self.assertAlmostEqual(agg["cost_usd"], 0.3)
        self.assertEqual(agg["input_side_total"], 150)
        self.assertEqual(agg["mean_turns"], 3.0)
        self.assertEqual(agg["graph_used"], 1)
        self.assertEqual(agg["mean_coverage"], 0.75)


class GradeWorktreeTest(unittest.TestCase):
    def test_expect_path_contains(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            path = root / "pkg" / "x.go"
            path.parent.mkdir(parents=True)
            path.write_text("Failed to read dashboards config from configDirectory\n")
            got = grade.grade_worktree(
                root,
                {
                    "expect_path_contains": [
                        {"path": "pkg/x.go", "needles": ["Failed to read dashboards config", "configDirectory"]}
                    ]
                },
            )
            self.assertTrue(got["ok"])

    def test_grafana_2task_ids_are_question_only(self) -> None:
        qs = grade.load_questions(Path(__file__).resolve().parent / "questions" / "grafana-2task.json")
        self.assertEqual([q["id"] for q in qs], ["provisioning-error-path", "provisioning-skip-log"])
        self.assertTrue(qs[1].get("expect_memory"))
        for q in qs:
            prompt = q["prompt"].lower()
            wrapped = grade.wrap_prompt(q["prompt"]).lower()
            self.assertNotIn("superopen", prompt)
            self.assertNotIn("so graph", prompt)
            self.assertNotIn("so memory", prompt)
            self.assertNotIn("use the graph", prompt)
            self.assertNotIn("use memory", wrapped)
            self.assertEqual(wrapped.strip(), q["prompt"].strip().lower())

    def test_grafana_memory_ids_are_question_only(self) -> None:
        qs = grade.load_questions(Path(__file__).resolve().parent / "questions" / "grafana-memory.json")
        self.assertEqual(
            [q["id"] for q in qs],
            ["provisioning-error-path", "provisioning-wrap-recall", "provisioning-load-path"],
        )
        self.assertTrue(qs[1].get("expect_memory"))
        self.assertFalse(qs[2].get("expect_memory"))
        self.assertGreaterEqual(len(qs[2]["key_facts"]), 3)
        for q in qs:
            prompt = q["prompt"].lower()
            wrapped = grade.wrap_prompt(q["prompt"]).lower()
            self.assertNotIn("superopen", prompt)
            self.assertNotIn("so graph", prompt)
            self.assertNotIn("so memory", prompt)
            self.assertNotIn("use the graph", prompt)
            self.assertNotIn("use memory", wrapped)
            self.assertEqual(wrapped.strip(), q["prompt"].strip().lower())

    def test_memory_used_transcript_and_markers(self) -> None:
        self.assertTrue(grade.memory_used("", {"memory_tools": 1}))
        self.assertTrue(grade.memory_used("", {"memory_injected": 1}))
        self.assertTrue(grade.memory_used("ran so memory get 12", None))
        self.assertFalse(grade.memory_used("fixed the wrap string", {"memory_tools": 0, "memory_injected": 0}))

    def test_default_index_timeout_grafana_memory(self) -> None:
        qs = Path(__file__).resolve().parent / "questions" / "grafana-memory.json"
        self.assertEqual(run_eval.default_index_timeout("grafana", qs), 10800)
        self.assertEqual(run_eval.default_index_timeout("grafana", Path("grafana.json")), 7200)
        self.assertEqual(run_eval.default_index_timeout("linux", None), 3600)

    def test_prepare_arms(self) -> None:
        self.assertEqual(set(run_eval.PREPARE.keys()), {"vanilla", "superopen"})

    def test_should_parallel_index(self) -> None:
        self.assertTrue(run_eval.should_parallel_index(["vanilla", "superopen"]))
        self.assertFalse(run_eval.should_parallel_index(["superopen"]))
        self.assertFalse(run_eval.should_parallel_index(["vanilla"]))

    def test_load_completed_arm(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            out = Path(raw)
            questions = [{"id": "q1"}, {"id": "q2"}]
            (out / "vanilla.totals.json").write_text("{}\n")
            (out / "vanilla.q1.metrics.json").write_text('{"turns": 1}\n')
            self.assertIsNone(run_eval.load_completed_arm(out, "vanilla", questions))
            (out / "vanilla.q2.metrics.json").write_text('{"turns": 2}\n')
            loaded = run_eval.load_completed_arm(out, "vanilla", questions)
            self.assertIsNotNone(loaded)
            self.assertEqual(loaded["q1"]["turns"], 1)

    def test_transcript_memory_injection(self) -> None:
        import transcripts

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "s.jsonl"
            path.write_text(
                json.dumps(
                    {
                        "type": "attachment",
                        "attachment": {
                            "type": "hook_success",
                            "hookEvent": "SessionStart",
                            "stdout": '{"hookSpecificOutput":{"additionalContext":"MEM #12 decision \\"JWT expiry\\""}}',
                        },
                    }
                )
                + "\n"
            )
            got = transcripts.count_jsonl_tools(path)
            self.assertGreaterEqual(got["memory_injected"], 1)
            self.assertTrue(grade.memory_used("", got))

    def test_grafana_2q_ids(self) -> None:
        qs = grade.load_questions(Path(__file__).resolve().parent / "questions" / "grafana-2q.json")
        self.assertEqual([q["id"] for q in qs], ["provisioning-dashboards", "grafana-live"])
        subset = grade.filter_questions(qs, "grafana-live")
        self.assertEqual([q["id"] for q in subset], ["grafana-live"])

    def test_docker_argv_does_not_mount_host_claude(self) -> None:
        import docker as eval_docker

        home = Path("/Users/evaluser")
        argv = eval_docker.run_argv(
            ["claude", "-p", "hi"],
            worktree=Path("/tmp/so-eval/worktrees/vanilla/repo"),
            claude_dir=Path("/tmp/so-eval/arms/vanilla/claude"),
            home=Path("/tmp/so-eval/arms/vanilla/home"),
            so_bin=None,
        )
        eval_docker.assert_isolated(argv, home=home)
        joined = " ".join(argv)
        self.assertNotIn(str(home / ".claude"), joined)
        self.assertNotIn(f"{home}:", joined)
        self.assertIn("--user", argv)
        self.assertIn("1000:1000", argv)

    def test_docker_argv_rejects_host_claude_mount(self) -> None:
        import docker as eval_docker

        home = Path("/Users/evaluser")
        argv = [
            "docker",
            "run",
            "-v",
            f"{home / '.claude'}:/eval/claude",
            "so-eval-claude",
            "claude",
        ]
        with self.assertRaises(RuntimeError):
            eval_docker.assert_isolated(argv, home=home)

    def test_transcript_tool_counts(self) -> None:
        import transcripts

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "s.jsonl"
            path.write_text(
                json.dumps(
                    {
                        "type": "assistant",
                        "message": {
                            "content": [
                                {"type": "tool_use", "name": "graph_query", "input": {}},
                                {"type": "tool_use", "name": "Grep", "input": {"pattern": "x"}},
                                {"type": "tool_use", "name": "Bash", "input": {"command": "rg Foo pkg"}},
                                {
                                    "type": "tool_use",
                                    "name": "Bash",
                                    "input": {"command": 'so memory search "dashboard provisioning"'},
                                },
                            ]
                        },
                    }
                )
                + "\n"
                + json.dumps(
                    {
                        "type": "assistant",
                        "message": {
                            "content": [
                                {
                                    "type": "tool_use",
                                    "name": "Bash",
                                    "input": {"command": 'so graph query "how does X work"'},
                                }
                            ]
                        },
                    }
                )
                + "\n"
            )
            got = transcripts.count_jsonl_tools(path)
            self.assertEqual(got["grep"], 1)
            self.assertEqual(got["bash_grep"], 1)
            self.assertGreaterEqual(got["graph_tools"], 2)
            self.assertGreaterEqual(got["memory_tools"], 1)


class DockerPathRewriteTest(unittest.TestCase):
    def test_rewrites_host_bins_home_and_claude(self) -> None:
        with tempfile.TemporaryDirectory(prefix="so-eval-", dir="/tmp") as raw:
            work = Path(raw)
            paths = isolate.arm_paths(work, "superopen")
            isolate.ensure_dirs(paths)
            plugin_hooks = paths["claude"] / "plugins" / "superopen-cc" / "hooks"
            plugin_hooks.mkdir(parents=True)
            (plugin_hooks / "hooks.json").write_text(
                json.dumps(
                    {
                        "hooks": {
                            "PreToolUse": [
                                {
                                    "hooks": [
                                        {
                                            "type": "command",
                                            "command": "/tmp/so coding hook --vendor=cc --event=PreToolUse --kind=search",
                                        }
                                    ]
                                }
                            ]
                        }
                    }
                )
            )
            marketplace = str(paths["home"] / ".local/share/superopen/claude-marketplace")
            (paths["claude"] / "settings.json").write_text(
                json.dumps(
                    {
                        "extraKnownMarketplaces": {
                            "superopen": {
                                "source": {"source": "directory", "path": marketplace}
                            }
                        }
                    }
                )
            )
            (paths["claude"] / "plugins" / "installed_plugins.json").write_text(
                json.dumps(
                    {
                        "plugins": {
                            "superopen-cc@superopen": {
                                "installPath": str(paths["claude"] / "plugins/cache/superopen/superopen-cc/0.1.0"),
                            }
                        }
                    }
                )
            )
            n = isolate.rewrite_docker_agent_paths(paths, so_bin=Path("/tmp/so"))
            self.assertGreater(n, 0)
            so_cmd = json.loads((plugin_hooks / "hooks.json").read_text())["hooks"]["PreToolUse"][0]["hooks"][0]["command"]
            self.assertTrue(so_cmd.startswith("/usr/local/bin/so coding hook"))
            self.assertNotIn("/tmp/so ", so_cmd)
            market = json.loads((paths["claude"] / "settings.json").read_text())
            mpath = market["extraKnownMarketplaces"]["superopen"]["source"]["path"]
            self.assertTrue(mpath.startswith("/eval/home/"))
            self.assertIn("claude-marketplace", mpath)
            self.assertNotIn(str(paths["home"]), mpath)
            installed = json.loads((paths["claude"] / "plugins" / "installed_plugins.json").read_text())
            ipath = installed["plugins"]["superopen-cc@superopen"]["installPath"]
            self.assertTrue(ipath.startswith("/eval/claude/"))
            self.assertNotIn("/usr/local/bin/so-eval", ipath)

    def test_hook_binary_missing_counts_and_fails_session(self) -> None:
        import transcripts

        with tempfile.TemporaryDirectory() as raw:
            work = Path(raw)
            paths = isolate.arm_paths(work, "superopen")
            isolate.ensure_dirs(paths)
            projects = paths["claude"] / "projects" / "-work"
            projects.mkdir(parents=True)
            session_id = "sess-hook-miss"
            (projects / f"{session_id}.jsonl").write_text(
                json.dumps(
                    {
                        "type": "attachment",
                        "attachment": {
                            "type": "hook_non_blocking_error",
                            "hookName": "PreToolUse:Bash",
                            "stderr": "Failed with non-blocking status code: /bin/sh: 1: /tmp/so: not found",
                            "exitCode": 127,
                            "command": "/tmp/so coding hook --vendor=cc --event=PreToolUse",
                        },
                    }
                )
                + "\n"
            )
            got = transcripts.count_jsonl_tools(projects / f"{session_id}.jsonl")
            self.assertGreater(got["hook_binary_missing"], 0)
            metrics = {"session_id": session_id}
            run_eval.record_session(metrics, paths, work / "out", "superopen", "q1")
            self.assertEqual(metrics["error"], "hook_binary_missing")
            self.assertGreater(metrics["transcript"]["hook_binary_missing"], 0)


if __name__ == "__main__":
    os.chdir(Path(__file__).resolve().parent)
    unittest.main()
