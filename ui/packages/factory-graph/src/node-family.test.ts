import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_NODE_FAMILIES,
  factoryGraphNodeFamilyDimensions,
  factoryGraphNodeFamilyForShellType,
  factoryGraphNodeFamilyRole,
  resolveFactoryGraphNodeDimensions,
  resolveFactoryGraphNodeResizeDimensions,
} from "./node-family.js";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: family sizing cases share one pure contract matrix.
describe("Factory graph node family roles", () => {
  it.each([
    ["constraint", "constraint", 156, 58],
    ["doc", "document", 168, 86],
    ["resource", "resource", 168, 86],
    ["worker", "worker", 156, 58],
    ["work-state", "work-state", 164, 86],
    ["work-type", "work-type", 156, 58],
    ["workstation", "workstation", 156, 196],
  ] as const)(
    "keeps %s family shape and defaults in the package contract",
    (family, shape, width, height) => {
      expect(factoryGraphNodeFamilyRole(family)).toMatchObject({
        defaultDimensions: { height, width },
        family,
        shape,
      });
      expect(factoryGraphNodeFamilyDimensions(family)).toEqual({
        height,
        width,
      });
    },
  );

  it("enumerates every family used by the semantic renderers", () => {
    expect(FACTORY_GRAPH_NODE_FAMILIES).toEqual([
      "constraint",
      "doc",
      "resource",
      "worker",
      "work-state",
      "work-type",
      "workstation",
    ]);
  });

  it.each([
    ["constraint", "constraint"],
    ["doc", "doc"],
    ["resource", "resource"],
    ["statePosition", "work-state"],
    ["worker", "worker"],
    ["workType", "work-type"],
    ["workstation", "workstation"],
  ] as const)("maps shell type %s to family %s", (nodeType, family) => {
    expect(factoryGraphNodeFamilyForShellType(nodeType)).toBe(family);
  });

  it("retains default and resolved dimensions as separate values", () => {
    expect(
      resolveFactoryGraphNodeDimensions("work-state", {
        height: 120,
        width: 240,
      }),
    ).toMatchObject({
      defaultDimensions: { height: 86, width: 164 },
      fittedDimensions: { height: 86, width: 164 },
      resolvedDimensions: { height: 120, width: 240 },
      source: "resolved",
    });
    expect(resolveFactoryGraphNodeDimensions("work-state")).toMatchObject({
      defaultDimensions: { height: 86, width: 164 },
      fittedDimensions: { height: 86, width: 164 },
      resolvedDimensions: { height: 86, width: 164 },
      source: "default",
    });
  });

  it.each([
    ["constraint", false, true],
    ["doc", true, true],
    ["resource", false, true],
    ["worker", false, true],
    ["work-state", false, true],
    ["work-type", false, true],
    ["workstation", true, true],
  ] as const)(
    "defines bounds and allowed axes for %s",
    (family, height, width) => {
      const role = factoryGraphNodeFamilyRole(family);
      expect(role.allowedAxes).toEqual({ height, width });
      expect(role.minimumDimensions.width).toBeLessThanOrEqual(
        role.defaultDimensions.width,
      );
      expect(role.minimumDimensions.height).toBeLessThanOrEqual(
        role.defaultDimensions.height,
      );
      expect(role.maximumDimensions.width).toBeGreaterThanOrEqual(
        role.defaultDimensions.width,
      );
      expect(role.maximumDimensions.height).toBeGreaterThanOrEqual(
        role.defaultDimensions.height,
      );
    },
  );

  it.each(FACTORY_GRAPH_NODE_FAMILIES)(
    "fits ordinary and localized content for the %s family",
    (family) => {
      const ordinary = resolveFactoryGraphNodeDimensions(family, {
        content: ["ordinary readable label with spaces"],
      });
      const localized = resolveFactoryGraphNodeDimensions(family, {
        content: ["工作流节点标签和路径"],
      });

      expect(ordinary.resolvedDimensions.width).toBeGreaterThanOrEqual(
        ordinary.minimumDimensions.width,
      );
      expect(ordinary.resolvedDimensions.width).toBeLessThanOrEqual(
        ordinary.maximumDimensions.width,
      );
      expect(localized.resolvedDimensions.width).toBeGreaterThanOrEqual(
        localized.minimumDimensions.width,
      );
      expect(localized.resolvedDimensions.height).toBeGreaterThanOrEqual(
        localized.minimumDimensions.height,
      );
    },
  );

  it("fits document paths and unbroken identifiers without ellipsis assumptions", () => {
    const document = resolveFactoryGraphNodeDimensions("doc", {
      content: [
        "docs/reference/factory-graph/this-is-a-long-authored-document-path.md",
      ],
    });
    const identifier = resolveFactoryGraphNodeDimensions("worker", {
      content: ["worker_with_a_very_long_unbroken_identifier_".repeat(8)],
    });

    expect(document.source).toBe("fitted");
    expect(document.resolvedDimensions.width).toBeGreaterThan(
      document.defaultDimensions.width,
    );
    expect(identifier.resolvedDimensions.width).toBe(
      identifier.maximumDimensions.width,
    );
    expect(identifier.resolvedDimensions.height).toBeGreaterThan(
      identifier.defaultDimensions.height,
    );
  });

  it("gives valid authored dimensions precedence and clamps them to family bounds", () => {
    const resolution = resolveFactoryGraphNodeDimensions("doc", {
      authoredDimensions: { height: 140, width: 260 },
      content: ["A long document path"],
    });
    const clamped = resolveFactoryGraphNodeDimensions("worker", {
      authoredDimensions: { height: 9999, width: 1 },
      content: ["worker"],
    });

    expect(resolution.resolvedDimensions).toEqual({ height: 140, width: 260 });
    expect(resolution.source).toBe("resolved");
    expect(clamped.resolvedDimensions).toEqual({
      height: clamped.maximumDimensions.height,
      width: clamped.minimumDimensions.width,
    });
  });

  it.each([
    { height: Number.NaN, width: 200 },
    { height: Number.POSITIVE_INFINITY, width: 200 },
    { height: 0, width: 200 },
    { height: 80, width: -1 },
  ])(
    "falls back deterministically for invalid authored dimensions: %o",
    (authoredDimensions) => {
      const resolution = resolveFactoryGraphNodeDimensions("work-state", {
        authoredDimensions,
        content: ["invalid-size-state"],
      });

      expect(resolution.source).toBe("fitted");
      expect(resolution.resolvedDimensions).toEqual(
        resolution.fittedDimensions,
      );
    },
  );

  it("keeps compact family fit height as the default while retaining authored size", () => {
    const resolution = resolveFactoryGraphNodeDimensions("resource", {
      authoredDimensions: { height: 200, width: 300 },
      content: ["resource-with-a-long-unbroken-name"],
    });

    expect(resolution.resolvedDimensions.width).toBe(300);
    expect(resolution.resolvedDimensions.height).toBe(200);
    expect(resolution.fittedDimensions.height).toBeLessThan(200);
  });

  it("normalizes interactive resize requests to the family's allowed axes", () => {
    expect(
      resolveFactoryGraphNodeResizeDimensions("resource", {
        height: 220,
        width: 9999,
      }),
    ).toEqual({
      height: factoryGraphNodeFamilyRole("resource").defaultDimensions.height,
      width: factoryGraphNodeFamilyRole("resource").maximumDimensions.width,
    });
    expect(
      resolveFactoryGraphNodeResizeDimensions("workstation", {
        height: 9999,
        width: 1,
      }),
    ).toEqual({
      height:
        factoryGraphNodeFamilyRole("workstation").maximumDimensions.height,
      width: factoryGraphNodeFamilyRole("workstation").minimumDimensions.width,
    });
  });
});
