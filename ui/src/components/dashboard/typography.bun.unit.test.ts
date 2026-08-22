import { describe, expect, it } from "bun:test";
import {
  DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES,
  DASHBOARD_RETIRED_TEXT_SIZE_LITERALS,
  DASHBOARD_TYPOGRAPHY_CONTRACT,
} from "../ui/dashboard-typography";

function getRole(
  role: (typeof DASHBOARD_TYPOGRAPHY_CONTRACT)[number]["role"],
): (typeof DASHBOARD_TYPOGRAPHY_CONTRACT)[number] {
  const entry = DASHBOARD_TYPOGRAPHY_CONTRACT.find(
    (item) => item.role === role,
  );
  if (!entry) {
    throw new Error(`expected ${role} typography role`);
  }
  return entry;
}

describe("dashboard typography contract", () => {
  it("documents Material scale mappings for shared dashboard roles", () => {
    expect(DASHBOARD_TYPOGRAPHY_CONTRACT.map((entry) => entry.role)).toEqual([
      "pageHeading",
      "sectionHeading",
      "bodyText",
      "supportingText",
    ]);
    expect(getRole("pageHeading")).toMatchObject({
      materialFamily: "display",
      materialVariant: "medium",
      textColorRole: "on-surface",
      typeUtilityClass: "type-display-medium",
    });
    expect(getRole("sectionHeading")).toMatchObject({
      materialFamily: "title",
      materialVariant: "large",
      textColorRole: "on-surface",
    });
    expect(getRole("bodyText")).toMatchObject({
      materialFamily: "body",
      materialVariant: "medium",
      textColorRole: "on-surface-variant",
    });
    expect(getRole("supportingText")).toMatchObject({
      materialFamily: "body",
      materialVariant: "small",
      textColorRole: "on-surface-variant",
    });
  });

  it("documents the code extension and label roles beside Material families", () => {
    expect(
      DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES.find(
        (entry) => entry.className === "af-body-code",
      ),
    ).toMatchObject({
      materialFamily: "code",
      textColorRole: "code",
      typeUtilityClass: "type-code-medium",
    });
    expect(
      DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES.find(
        (entry) => entry.className === "af-supporting-label",
      ),
    ).toMatchObject({
      materialFamily: "label",
      textColorRole: "on-surface-subtle",
    });
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
