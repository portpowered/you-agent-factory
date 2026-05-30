import { describe, expect, it } from "vitest";

import type { FactorySessionTarget } from "../factory-sessions";
import {
  allocateFirstFreeSuffixedFactoryName,
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
} from "./import-save-mode";

describe("extractNamedFactoryNamesFromSessionTargets", () => {
  it("returns an empty list when targets are missing or empty", () => {
    expect(extractNamedFactoryNamesFromSessionTargets(undefined)).toEqual([]);
    expect(extractNamedFactoryNamesFromSessionTargets([])).toEqual([]);
  });

  it("collects trimmed named factory refs and ignores non-named targets", () => {
    const targets: FactorySessionTarget[] = [
      { ref: { kind: "named", name: "  beta  " } },
      { ref: { kind: "named", name: "alpha" } },
      { ref: { kind: "named", name: "alpha" } },
      { ref: { kind: "named", name: "   " } },
      { ref: { kind: "bundled", path: "ignored" } },
    ];

    expect(extractNamedFactoryNamesFromSessionTargets(targets)).toEqual(["alpha", "beta"]);
  });
});

describe("allocateFirstFreeSuffixedFactoryName", () => {
  it("returns the preferred name when it is unused", () => {
    expect(allocateFirstFreeSuffixedFactoryName("Factory A", ["Other"])).toBe("Factory A");
  });

  it("returns the raw preferred value when it is empty after trim", () => {
    expect(allocateFirstFreeSuffixedFactoryName("   ", ["Other"])).toBe("   ");
  });

  it("allocates the first free numeric suffix on collision", () => {
    expect(allocateFirstFreeSuffixedFactoryName("Factory A", ["Factory A", "Factory A-2"])).toBe(
      "Factory A-3",
    );
  });
});

describe("resolveImportCreateFactoryName", () => {
  it("allocates the next suffixed name when the preferred name collides", () => {
    expect(resolveImportCreateFactoryName("alpha", ["alpha", "beta"])).toEqual({
      factoryName: "alpha-2",
      replacesExisting: false,
    });
  });

  it("marks replacesExisting when the resolved factory name is already listed", () => {
    expect(resolveImportCreateFactoryName("", [""])).toEqual({
      factoryName: "",
      replacesExisting: true,
    });
  });
});
