import importlib.util
import json
import sys
import tempfile
import unittest
from argparse import Namespace
from pathlib import Path
from unittest import mock

SCRIPT = Path(__file__).with_name("release-packages.py")
sys.dont_write_bytecode = True
SPEC = importlib.util.spec_from_file_location("release", SCRIPT)
release = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
SPEC.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.config = release.load_config()

    def test_all_supported_tags(self):
        expected = {"cli-1.0.0": ("cli", "1.0.0")}
        for tag, parsed in expected.items():
            key, version, _ = release.parse_tag(tag, self.config)
            self.assertEqual((key, version), parsed)

    def test_workflow_inventory_and_publishers_are_standardized(self):
        workflow_directory = release.ROOT / ".github" / "workflows"
        expected = {
            "admin-pr-management.yml",
            "admin-pr-summary.yml",
            "ci-automation.yml",
            "ci-cli.yml",
            "ci-cross-platform.yml",
            "ci-web.yml",
            "release-cli.yml",
            "release-packages.yml",
        }
        actual = {path.name for path in workflow_directory.iterdir()}
        self.assertEqual(actual, expected)

        orchestrator = (workflow_directory / "release-packages.yml").read_text(encoding="utf-8")
        for tool in self.config.values():
            publisher = tool["publisher"]
            self.assertIn(f"./.github/workflows/release-{publisher}.yml", orchestrator)
            self.assertIn(f"- {tool['display_name']}", orchestrator)

        pr_summary = (workflow_directory / "admin-pr-summary.yml").read_text(encoding="utf-8")
        self.assertIn("pull-requests: write", pr_summary)
        self.assertNotIn("issues: write", pr_summary)
        self.assertIn("workflow_dispatch:", pr_summary)
        self.assertIn("pr_number:", pr_summary)
        self.assertIn("environment: release-notes", pr_summary)

        pr_management = (workflow_directory / "admin-pr-management.yml").read_text(
            encoding="utf-8"
        )
        self.assertIn("environment: release-notes", pr_management)
        self.assertIn("OPENROUTER_API_KEY is not configured", pr_management)
        self.assertIn("Manual PR title required", pr_management)
        self.assertIn("superopen-pr-title-help", pr_management)
        self.assertIn("type(scope): lowercase summary", pr_management)

    def test_scripts_are_named_after_their_workflow(self):
        # Each script shares a stem with the workflow that owns it, and tests
        # are that stem prefixed with test_ (dashes become underscores).
        script_directory = release.ROOT / ".github" / "scripts"
        workflow_stems = {
            path.stem for path in (release.ROOT / ".github" / "workflows").iterdir()
        }
        for path in script_directory.glob("*.py"):
            stem = path.stem
            if stem.startswith("test_"):
                owner = stem[len("test_"):].replace("_", "-")
                self.assertTrue(
                    (script_directory / f"{owner}.py").exists(),
                    f"{path.name} has no matching script {owner}.py",
                )
                continue
            self.assertIn(
                stem,
                workflow_stems,
                f"{path.name} does not match a workflow name",
            )

    def test_release_cli_is_build_only(self):
        # release-packages.yml owns tagging, notes, the GitHub Release, and the
        # Homebrew bump. A tag trigger here would double-publish.
        workflow = (release.ROOT / ".github" / "workflows" / "release-cli.yml").read_text(
            encoding="utf-8"
        )
        self.assertIn("workflow_call:", workflow)
        self.assertNotIn("on:\n  push:", workflow)
        self.assertNotIn("gh release create", workflow)
        self.assertNotIn("action-gh-release", workflow)
        self.assertNotIn("action-homebrew-bump-formula", workflow)
        self.assertIn("release-assets-", workflow)

    def test_labeler_config_covers_every_release_path_root(self):
        labeler = (release.ROOT / ".github" / "labeler.yml").read_text(encoding="utf-8")
        for pattern in self.config["cli"]["paths"]:
            root = pattern.split("/", 1)[0].replace("**", "").strip()
            if root and not root.endswith((".mod", ".sum")) and root != "Makefile":
                self.assertIn(root, labeler, f"labeler.yml does not mention {root}")

    def test_tool_and_version_construct_release_tag(self):
        tag, key, version, tool = release.resolve_release_identity(
            self.config, tool_key="cli", version="0.2.0"
        )
        self.assertEqual(tag, "cli-0.2.0")
        self.assertEqual((key, version, tool["publisher"]), ("cli", "0.2.0", "cli"))

        tag, key, version, tool = release.resolve_release_identity(
            self.config, tool_key="CLI", version="0.2.0"
        )
        self.assertEqual(tag, "cli-0.2.0")
        self.assertEqual((key, version, tool["release_name"]), ("cli", "0.2.0", "cli"))

    def test_tool_and_version_reject_unknown_tool_or_invalid_version(self):
        with self.assertRaises(release.ReleaseError):
            release.resolve_release_identity(self.config, tool_key="unknown", version="1.2.3")
        with self.assertRaises(release.ReleaseError):
            release.resolve_release_identity(self.config, tool_key="cli", version="v1.2.3")

    def test_rejects_unstable_or_malformed_tags(self):
        for tag in ("cli-1.2", "cli-v1.2.3", "cli-1.2.3-rc.1", "unknown-1.2.3"):
            with self.subTest(tag=tag), self.assertRaises(release.ReleaseError):
                release.parse_tag(tag, self.config)

    def test_path_matching_includes_renames(self):
        self.assertTrue(release.path_matches("docs/old.md", ["internal/**"], "internal/old.go"))
        self.assertFalse(release.path_matches("docs/guide.md", ["internal/**"]))

    def test_summary_validation_requires_exact_pr_set(self):
        valid = {"items": [{"number": 10, "category": "Fixes", "summary": "Corrects streaming output."}]}
        self.assertEqual(release.validate_summaries(valid, {10})[0]["number"], 10)
        with self.assertRaises(release.ReleaseError):
            release.validate_summaries(valid, {10, 11})
        with self.assertRaises(release.ReleaseError):
            release.validate_summaries({"items": valid["items"] * 2}, {10})

    def test_component_summary_validation_requires_exact_component_set(self):
        value = {"items": [{"tool": "cli", "category": "Fixes", "summary": "Corrects hook capture."}]}
        validated = release.validate_component_summaries(value, {"cli"})
        self.assertEqual(set(validated), {"cli"})
        with self.assertRaises(release.ReleaseError):
            release.validate_component_summaries(value, {"cli", "other"})

    def test_merged_pr_summary_cache_round_trips(self):
        pr = {"number": 10, "merge_commit_sha": "a" * 40}
        components = {
            "cli": {
                "tool_name": "Superopen CLI",
                "category": "Features",
                "summary": "Adds session porting for Codex.",
            }
        }
        body = release.render_summary_comment(pr, components, self.config)
        decoded = release.decode_summary_cache(body, 10, "a" * 40, self.config)
        self.assertEqual(decoded, components)
        self.assertIn("Superopen CLI · Features", body)

    def test_summary_cache_rejects_the_wrong_merge_commit(self):
        pr = {"number": 10, "merge_commit_sha": "a" * 40}
        components = {
            "cli": {
                "tool_name": "Superopen CLI",
                "category": "Fixes",
                "summary": "Corrects streaming output.",
            }
        }
        body = release.render_summary_comment(pr, components, self.config)
        with self.assertRaisesRegex(release.ReleaseError, "merge commit"):
            release.decode_summary_cache(body, 10, "b" * 40, self.config)

    def test_summary_cache_rejects_changed_tool_path_configuration(self):
        pr = {"number": 10, "merge_commit_sha": "a" * 40}
        components = {
            "cli": {
                "tool_name": "Superopen CLI",
                "category": "Fixes",
                "summary": "Corrects streaming output.",
            }
        }
        body = release.render_summary_comment(pr, components, self.config)
        changed_config = json.loads(json.dumps(self.config))
        changed_config["cli"]["paths"] = ["different/**"]
        with self.assertRaisesRegex(release.ReleaseError, "configuration"):
            release.decode_summary_cache(body, 10, "a" * 40, changed_config)

    def test_summary_cache_ignores_non_bot_comments(self):
        pr = {"number": 10, "merge_commit_sha": "a" * 40}
        components = {
            "cli": {
                "tool_name": "Superopen CLI",
                "category": "Fixes",
                "summary": "Corrects streaming output.",
            }
        }
        body = release.render_summary_comment(pr, components, self.config)

        class UserCommentGitHub:
            @staticmethod
            def paginated(_endpoint, **_kwargs):
                return [{"user": {"login": "contributor", "type": "User"}, "body": body}]

        self.assertIsNone(
            release.load_cached_pr_components(UserCommentGitHub(), 10, "a" * 40, self.config)
        )

    def test_release_uses_bot_cache_without_fetching_pr_files(self):
        pr = {
            "number": 10,
            "title": "feat: add porting",
            "html_url": "https://github.test/pulls/10",
            "merged_at": "2026-08-03T00:00:00Z",
            "merge_commit_sha": "a" * 40,
            "base": {"ref": "main"},
        }
        components = {
            "cli": {
                "tool_name": "Superopen CLI",
                "category": "Features",
                "summary": "Adds session porting for Codex.",
            }
        }
        cache_body = release.render_summary_comment(pr, components, self.config)

        class CachedGitHub:
            def get(self, endpoint):
                if endpoint == "/commits/commit-sha/pulls":
                    return [pr]
                raise AssertionError(f"unexpected GET: {endpoint}")

            def paginated(self, endpoint, **_kwargs):
                if endpoint == "/issues/10/comments":
                    return [
                        {
                            "user": {"login": "github-actions[bot]", "type": "Bot"},
                            "body": cache_body,
                        }
                    ]
                raise AssertionError(f"release should not fetch PR files: {endpoint}")

        with mock.patch.object(release, "run", return_value="commit-sha"):
            prs, direct = release.collect_prs(
                CachedGitHub(),
                "cli-0.1.0",
                "source-sha",
                self.config["cli"]["paths"],
                tool_key="cli",
                config=self.config,
            )
        self.assertEqual(direct, [])
        self.assertEqual(prs[0]["cached_summary"], components["cli"])

    def test_invalid_cache_falls_back_to_bounded_file_evidence(self):
        pr = {
            "number": 10,
            "title": "fix: correct hook",
            "body": "details",
            "html_url": "https://github.test/pulls/10",
            "merged_at": "2026-08-03T00:00:00Z",
            "merge_commit_sha": "a" * 40,
            "base": {"ref": "main"},
            "user": {"login": "contributor"},
        }
        stale_pr = {**pr, "merge_commit_sha": "b" * 40}
        components = {
            "cli": {
                "tool_name": "Superopen CLI",
                "category": "Fixes",
                "summary": "Corrects hook capture.",
            }
        }
        stale_body = release.render_summary_comment(stale_pr, components, self.config)

        class FallbackGitHub:
            file_api_used = False

            def get(self, endpoint):
                if endpoint == "/commits/commit-sha/pulls":
                    return [pr]
                raise AssertionError(f"unexpected GET: {endpoint}")

            def paginated(self, endpoint, **_kwargs):
                if endpoint == "/issues/10/comments":
                    return [
                        {
                            "user": {"login": "github-actions[bot]", "type": "Bot"},
                            "body": stale_body,
                        }
                    ]
                if endpoint == "/pulls/10/files":
                    self.file_api_used = True
                    return [
                        {
                            "filename": "internal/coding/hook/hook.go",
                            "status": "modified",
                            "additions": 2,
                            "deletions": 1,
                            "patch": "+fix",
                        }
                    ]
                raise AssertionError(f"unexpected pagination: {endpoint}")

        github = FallbackGitHub()
        with mock.patch.object(release, "run", return_value="commit-sha"):
            prs, _ = release.collect_prs(
                github,
                "cli-0.1.0",
                "source-sha",
                self.config["cli"]["paths"],
                tool_key="cli",
                config=self.config,
            )
        self.assertTrue(github.file_api_used)
        self.assertNotIn("cached_summary", prs[0])
        self.assertEqual(prs[0]["files"][0]["filename"], "internal/coding/hook/hook.go")

    def test_component_evidence_suppresses_cross_component_body_and_noisy_patches(self):
        pr = {
            "number": 11,
            "title": "chore: bump deps",
            "body": "This also rewrites the web UI.",
            "html_url": "https://github.test/pulls/11",
            "merged_at": "2026-08-03T00:00:00Z",
            "user": {"login": "contributor"},
        }
        files = [
            {"filename": "go.sum", "status": "modified", "additions": 9, "deletions": 9, "patch": "+noise"},
            {"filename": "docs/guide.md", "status": "modified", "additions": 1, "deletions": 0, "patch": "+doc"},
        ]
        evidence = release.build_pr_evidence(pr, files, ["go.sum"])
        self.assertEqual(evidence["body"], "")
        self.assertEqual(evidence["files"][0]["patch"], "")
        self.assertTrue(evidence["change_summary"]["has_unscoped_changes"])

    def test_release_notes_include_deduplicated_human_contributors(self):
        prs = [
            {"number": 1, "author": "Alice", "html_url": "https://github.test/pulls/1"},
            {"number": 2, "author": "alice", "html_url": "https://github.test/pulls/2"},
            {"number": 3, "author": "dependabot[bot]", "html_url": "https://github.test/pulls/3"},
        ]
        summaries = [
            {"number": 1, "category": "Features", "summary": "Adds a flag."},
            {"number": 2, "category": "Fixes", "summary": "Fixes a crash."},
            {"number": 3, "category": "Dependencies", "summary": "Bumps a dependency."},
        ]
        notes = release.render_notes("Superopen CLI", "0.2.0", prs, summaries)
        self.assertIn("# Superopen CLI 0.2.0", notes)
        self.assertIn("@Alice", notes)
        self.assertNotIn("dependabot", notes.split("## Contributors")[1])

    def test_summary_rejects_links_and_unknown_categories(self):
        with self.assertRaises(release.ReleaseError):
            release.validate_summaries(
                {"items": [{"number": 1, "category": "Fixes", "summary": "See https://example.test"}]},
                {1},
            )
        with self.assertRaises(release.ReleaseError):
            release.validate_summaries(
                {"items": [{"number": 1, "category": "Unknown", "summary": "Corrects output."}]},
                {1},
            )

    def test_summary_validation_explains_forbidden_dependency_constraint(self):
        with self.assertRaisesRegex(release.ReleaseError, "forbidden character"):
            release.validate_summaries(
                {"items": [{"number": 1, "category": "Dependencies", "summary": "Requires <2.0.0"}]},
                {1},
            )

    def test_version_file_and_go_constant_must_agree(self):
        def fake_run(*args, **_kwargs):
            target = args[-1]
            if target.endswith(":VERSION"):
                return "0.1.0"
            return 'var Version = "0.9.9"'

        tool = self.config["cli"]
        with mock.patch.object(release, "run", side_effect=fake_run):
            with self.assertRaisesRegex(release.ReleaseError, "disagree"):
                release.persisted_version_at(tool, "sha")

    def test_persisted_version_accepts_previous_or_exact_requested_version(self):
        self.assertFalse(release.version_already_set("0.1.0", "0.1.0", "0.2.0", "CLI"))
        self.assertTrue(release.version_already_set("0.2.0", "0.1.0", "0.2.0", "CLI"))
        with self.assertRaises(release.ReleaseError):
            release.version_already_set("0.5.0", "0.1.0", "0.2.0", "CLI")

    def test_version_file_rejects_non_semver(self):
        with self.assertRaises(release.ReleaseError):
            release.extract_version_file("v0.1.0\n")
        self.assertEqual(release.extract_version_file(" 0.1.0\n"), "0.1.0")

    def test_version_updates_rewrite_both_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "VERSION").write_text("0.1.0\n", encoding="utf-8")
            go_file = root / "version.go"
            go_file.write_text(
                'package version\n\nvar Version = "0.1.0"\n\nvar Commit = ""\n', encoding="utf-8"
            )
            with mock.patch.object(release, "ROOT", root):
                release.apply_version_files(["VERSION", "version.go"], "0.2.0")
                self.assertEqual((root / "VERSION").read_text(encoding="utf-8"), "0.2.0\n")
                self.assertIn('var Version = "0.2.0"', go_file.read_text(encoding="utf-8"))
                release.verify_version_files(["VERSION", "version.go"], "0.2.0")
                with self.assertRaises(release.ReleaseError):
                    release.verify_version_files(["VERSION", "version.go"], "0.3.0")

    def test_go_version_update_requires_a_unique_match(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "version.go"
            path.write_text("package version\n", encoding="utf-8")
            with self.assertRaisesRegex(release.ReleaseError, "uniquely"):
                release.replace_go_version(path, "0.2.0")

    def test_bump_rejects_changes_outside_configured_version_files(self):
        with tempfile.TemporaryDirectory() as directory:
            metadata = Path(directory) / "release-metadata.json"
            metadata.write_text(
                json.dumps(
                    {
                        "version_strategy": "version-file",
                        "version": "0.2.0",
                        "version_files": ["VERSION"],
                        "resume_sha": None,
                        "version_already_set": False,
                    }
                ),
                encoding="utf-8",
            )
            with mock.patch.object(release, "apply_version_files"), mock.patch.object(
                release, "run", return_value="VERSION\nunexpected.go"
            ), self.assertRaisesRegex(release.ReleaseError, "only configured files"):
                release.bump(Namespace(metadata=str(metadata)))

    def test_paginator_stops_at_explicit_item_limit(self):
        # The paginator narrows the final page to the remaining budget, so a
        # 150-item cap must cost exactly two requests and never overfetch.
        class CountingGitHub(release.GitHub):
            def __init__(self):
                super().__init__("owner/repo", "")
                self.requested_sizes = []

            def get(self, endpoint, *, allow_404=False):
                size = int(endpoint.split("per_page=")[1].split("&")[0])
                self.requested_sizes.append(size)
                return [{"filename": f"file{index}"} for index in range(size)]

        github = CountingGitHub()
        items = github.paginated("/pulls/1/files", max_items=150)
        self.assertEqual(len(items), 150)
        self.assertEqual(github.requested_sizes, [100, 50])

    def test_model_fallback_is_bounded_for_workflow_timeout(self):
        router = release.OpenRouter("key", "primary", "fallback")
        attempts = []

        def always_fail(*_args, **_kwargs):
            attempts.append(1)
            raise release.ReleaseError("rejected")

        with mock.patch.object(release, "extract_json", side_effect=always_fail), mock.patch(
            "urllib.request.urlopen"
        ) as urlopen:
            urlopen.return_value.__enter__.return_value = mock.Mock()
            with mock.patch("json.load", return_value={"choices": [{"message": {"content": "{}"}}]}):
                with self.assertRaises(release.ReleaseError):
                    router.complete("prompt", lambda value: value)
        self.assertEqual(len(attempts), 2 * release.MODEL_ATTEMPTS_PER_MODEL)

    def test_run_rejects_commands_outside_the_allowlist(self):
        with self.assertRaisesRegex(release.ReleaseError, "allowlisted"):
            release.run("curl", "https://example.test")


if __name__ == "__main__":
    unittest.main(verbosity=2)
