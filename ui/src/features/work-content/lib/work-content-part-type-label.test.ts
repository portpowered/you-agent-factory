import { describe, expect, it } from "vitest";
import { workContentPartTypeLabel } from "./work-content-part-type-label";

describe("workContentPartTypeLabel", () => {
  it("labels canonical lowercase and legacy uppercase API part types", () => {
    expect(workContentPartTypeLabel("text")).toBe("Text");
    expect(workContentPartTypeLabel("TEXT")).toBe("Text");
    expect(workContentPartTypeLabel("JSON")).toBe("JSON");
    expect(workContentPartTypeLabel("IMAGE")).toBe("Image");
    expect(workContentPartTypeLabel("AUDIO")).toBe("Audio");
    expect(workContentPartTypeLabel("BINARY")).toBe("Binary");
  });

  it("falls back to the raw type when no label is registered", () => {
    expect(workContentPartTypeLabel("CUSTOM")).toBe("CUSTOM");
    expect(workContentPartTypeLabel(undefined)).toBe("Content");
  });
});
