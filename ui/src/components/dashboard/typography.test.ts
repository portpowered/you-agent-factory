import { describe, it, expect } from "vitest";
import {
  DASHBOARD_RETIRED_TEXT_SIZE_LITERALS,
  DASHBOARD_TYPOGRAPHY_CONTRACT,
} from "./typography";

function getRole(
  role: (typeof DASHBOARD_TYPOGRAPHY_CONTRACT)[number]["role"],
): (typeof DASHBOARD_TYPOGRAPHY_CONTRACT)[number] {
  const entry = DASHBOARD_TYPOGRAPHY_CONTRACT.find((item) => item.role === role);
  if (!entry) {
    throw new Error(`expected ${role} typography role`);
  }
  return entry;
}

describe("dashboard typography contract", () => {
  it("documents the shared semantic roles and their current dashboard mappings", () => {
    expect(DASHBOARD_TYPOGRAPHY_CONTRACT.map((entry) => entry.role)).toEqual([
      "pageHeading",
      "sectionHeading",
      "bodyText",
      "supportingText",
    ]);
    expect(getRole("pageHeading").usage).toContain("page title");
    expect(getRole("sectionHeading").usage).toEqual(
      expect.arrayContaining(["widget title", "detail section heading"]),
    );
    expect(getRole("bodyText").usage).toEqual(
      expect.arrayContaining(["detail copy", "table body text", "trace metadata"]),
    );
    expect(getRole("supportingText").usage).toEqual(
      expect.arrayContaining(["metadata labels", "chart-axis/supporting labels"]),
    );
  });

  it("retires the repeated dashboard-only size literals for the covered roles", () => {
    expect(DASHBOARD_RETIRED_TEXT_SIZE_LITERALS).toEqual([
      "text-[0.78rem]",
      "text-[0.72rem]",
      "text-[0.74rem]",
      "text-[0.68rem]",
    ]);
    expect(getRole("bodyText").replacedLiterals).toEqual(["text-[0.78rem]"]);
    expect(getRole("supportingText").replacedLiterals).toEqual([
      "text-[0.72rem]",
      "text-[0.74rem]",
      "text-[0.68rem]",
    ]);
  });

  it("raises body and supporting roles above the prior shared dashboard baseline", () => {
    expect(getRole("bodyText").minimumRem).toBeGreaterThan(0.78);
    expect(getRole("supportingText").minimumRem).toBeGreaterThan(0.78);
  });
});

