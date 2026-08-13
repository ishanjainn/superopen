import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import { afterEach, describe, expect, it } from "vitest";
import { listOrRead, writeHarnessFile } from "./files";

const roots: string[] = [];

afterEach(() => {
	delete process.env.SUPEROPEN_ROOT;
	for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

describe("v2 single-file policy paths", () => {
	it("uses canonical root files and rejects legacy nested aliases", () => {
		const root = mkdtempSync(join(tmpdir(), "superopen-files-v2-"));
		roots.push(root);
		process.env.SUPEROPEN_ROOT = root;
		mkdirSync(join(root, ".so"), { recursive: true });
		writeFileSync(join(root, ".so", "guardrails.yaml"), "# guardrails\n");
		writeFileSync(join(root, ".so", "evals.yaml"), "# evals\n");

		expect(listOrRead("guardrails.yaml")?.body).toBe("# guardrails\n");
		expect(listOrRead("evals.yaml")?.body).toBe("# evals\n");
		expect(listOrRead("guardrails/guardrails.yaml")).toBeNull();
		expect(listOrRead("evals/configs.yaml")).toBeNull();
		expect(listOrRead("rules/cursor/testing.mdc")).toBeNull();
		expect(listOrRead("skills/codex/testing/SKILL.md")).toBeNull();
		expect(() => writeHarnessFile("guardrails/guardrails.yaml", "x")).toThrow();
	})
});
