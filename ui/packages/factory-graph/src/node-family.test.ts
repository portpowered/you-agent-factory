import { describe, expect, it } from "vitest";

import {
  FACTORY_GRAPH_NODE_FAMILIES,
  factoryGraphNodeFamilyDimensions,
  factoryGraphNodeFamilyForShellType,
  factoryGraphNodeFamilyRole,
  resolveFactoryGraphNodeDimensions,
} from "./node-family.js";

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
      expect(factoryGraphNodeFamilyRole(family)).toEqual({
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
    ).toEqual({
      defaultDimensions: { height: 86, width: 164 },
      resolvedDimensions: { height: 120, width: 240 },
      source: "resolved",
    });
    expect(resolveFactoryGraphNodeDimensions("work-state")).toEqual({
      defaultDimensions: { height: 86, width: 164 },
      resolvedDimensions: { height: 86, width: 164 },
      source: "default",
    });
  });
});
