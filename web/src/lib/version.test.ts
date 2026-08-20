import { describe, expect, it } from "vitest";
import { VERSION, displayVersion, parseSoVersion } from "./version";

describe("version", () => {
  it("is semver MAJOR.MINOR.PATCH", () => {
    expect(VERSION).toMatch(/^\d+\.\d+\.\d+$/);
  });

  it("formats display without a leading v", () => {
    expect(displayVersion("0.1.0")).toBe("0.1.0");
    expect(displayVersion("v1.2.3")).toBe("1.2.3");
    expect(displayVersion()).toBe(VERSION);
  });

  it("parses so version stdout", () => {
    expect(parseSoVersion("so 0.3.0\n")).toBe("0.3.0");
    expect(parseSoVersion("so 0.3.0 (abc1234)")).toBe("0.3.0");
    expect(parseSoVersion("")).toBeNull();
  });
});
