import { describe, expect, it } from "vitest";
import { recommendationTypeLabel } from "./recommendation-type-label";

describe("recommendationTypeLabel", () => {
  it("maps docs to Knowledge", () => {
    expect(recommendationTypeLabel("docs")).toBe("Knowledge");
    expect(recommendationTypeLabel("doc")).toBe("Knowledge");
    expect(recommendationTypeLabel("knowledge")).toBe("Knowledge");
  });

  it("title-cases known types", () => {
    expect(recommendationTypeLabel("guardrail")).toBe("Guardrail");
    expect(recommendationTypeLabel("skill")).toBe("Skill");
    expect(recommendationTypeLabel("graph")).toBe("Graph");
  });

  it("falls back for unknown / empty", () => {
    expect(recommendationTypeLabel()).toBe("Rec");
    expect(recommendationTypeLabel("")).toBe("Rec");
    expect(recommendationTypeLabel("custom")).toBe("custom");
  });
});
