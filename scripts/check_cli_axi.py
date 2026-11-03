#!/usr/bin/env python3
"""Exercise the user-facing CLI's AXI contracts against a built binary."""

import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path


def run(binary, *args, cwd=None):
    result = subprocess.run(
        [str(binary), *args],
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=20,
        check=False,
    )
    return result.returncode, result.stdout, result.stderr


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def require_compact_tables(output, name):
    for fields in re.findall(r"\w+\[\d+\]\{([^}]+)\}:", output):
        require(len(fields.split(",")) <= 4,
                f"{name}: default table has more than four fields: {fields}")


def command_paths(binary):
    pending = [()]
    seen = set()
    while pending:
        path = pending.pop(0)
        if path in seen:
            continue
        seen.add(path)
        code, output, _ = run(binary, *path, "--help")
        name = "so " + " ".join(path)
        require(code == 0, f"{name} --help exited {code}")
        require(all(flag in output for flag in ("Usage:", "--help", "--json", "--full")),
                f"{name} --help is missing usage or AXI flags")
        require(output.count("\n") <= 60, f"{name} --help is too long")
        commands = re.search(r"^Available Commands:\n((?:  \S+.*\n)+)", output, re.M)
        if commands:
            for line in commands.group(1).splitlines():
                child = line.strip().split()[0]
                if child not in ("help", "completion"):
                    pending.append((*path, child))
    return seen


def audit_errors(binary, paths):
    for path in sorted(paths):
        name = "so " + " ".join(path)
        code, output, _ = run(binary, *path, "--definitely-not-a-flag")
        require(code == 2, f"{name}: unknown flag exited {code}, expected 2")
        require(output.startswith("error: ") and "help[1]:\n  usage: " in output,
                f"{name}: unknown flag lacks structured error and usage hint: {output!r}")

        code, output, _ = run(binary, "--json", *path, "--definitely-not-a-flag")
        require(code == 2, f"{name} --json: unknown flag exited {code}, expected 2")
        try:
            error = json.loads(output)
        except json.JSONDecodeError as exc:
            raise AssertionError(f"{name}: invalid JSON error: {output!r}") from exc
        require(error.get("ok") is False and error.get("code") == 2
                and error.get("error") and error.get("hint"),
                f"{name}: incomplete JSON error: {error!r}")


def audit_output(binary):
    with tempfile.TemporaryDirectory(prefix="so-axi-") as directory:
        root = Path(directory)
        (root / "go.mod").write_text("module example.com/axi\n\ngo 1.23\n")
        (root / "main.go").write_text(
            'package main\nfunc main() { Greet("world") }\n'
            'func Greet(name string) string { return "hello " + name }\n'
        )
        (root / "main_test.go").write_text(
            'package main\nimport "testing"\n'
            'func TestGreet(t *testing.T) { _ = Greet("world") }\n'
        )
        subprocess.run(["git", "init", "-q"], cwd=root, check=True)
        subprocess.run(["git", "add", "go.mod", "main.go", "main_test.go"], cwd=root, check=True)

        code, output, _ = run(binary, "init", "--root", str(root), cwd=root)
        require(code == 0 and "init: done" in output and "help[" in output,
                f"init lacks status or next step: {output!r}")
        require_compact_tables(output, "init")

        for args, expected in (
            (("graph", "status"), "state: ready"),
            (("graph", "query", "What calls Greet?"), "callers["),
            (("graph", "code-search", "--", "hello "), "Greet"),
            (("sessions", "list"), "count: 0"),
            (("harvest", "list"), "count: 0"),
            (("scan",), "count: 0"),
            (("blocks", "scan"), "count: 0"),
        ):
            code, output, _ = run(binary, "--root", str(root), *args, cwd=root)
            require(code == 0 and expected in output,
                    f"so {' '.join(args)} lacks {expected!r}: {output!r}")
            require_compact_tables(output, "so " + " ".join(args))

        for group in ((), ("graph",), ("memory",), ("sessions",), ("harvest",), ("blocks",)):
            code, output, _ = run(binary, *group, "--root", str(root), cwd=root)
            require(code == 0 and output.strip() and not output.startswith("Usage:")
                    and "Available Commands:" not in output,
                    f"so {' '.join(group)} has no content-first home: {output!r}")
            require_compact_tables(output, "so " + " ".join(group))

        code, output, _ = run(binary, "memory", "capture", "--kind", "knowledge",
                              "--horizon", "long", "--title", "Greeting contract",
                              "--text", "Greet returns hello followed by the name.",
                              "--root", str(root), cwd=root)
        require(code == 0 and "captured: ok" in output, f"memory capture failed: {output!r}")
        code, output, _ = run(binary, "memory", "recall", "greeting",
                              "--root", str(root), cwd=root)
        require(code == 0 and "Greeting contract" in output and "Greet returns hello" in output,
                f"memory recall lacks the captured body: {output!r}")

        code, output, _ = run(binary, "--json", "sessions", "list", "--root", str(root), cwd=root)
        require(code == 0, f"sessions list --json exited {code}")
        data = json.loads(output)
        require(data.get("ok") is True and data.get("count") == 0,
                f"sessions list --json lacks the AXI envelope: {data!r}")


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: check_cli_axi.py <so-binary>")
    binary = Path(sys.argv[1]).resolve()
    require(binary.is_file(), f"missing CLI binary: {binary}")
    paths = command_paths(binary)
    require(len(paths) >= 80, f"CLI command catalog unexpectedly shrank to {len(paths)}")
    audit_errors(binary, paths)
    audit_output(binary)
    print(f"AXI CLI audit passed: {len(paths)} commands, text/JSON errors, and live output")


if __name__ == "__main__":
    main()
