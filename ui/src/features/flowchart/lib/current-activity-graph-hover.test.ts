import { describe, expect, it } from "vitest";

import {
  CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS,
  CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS,
  currentActivityGraphEdgeHoverClassName,
  currentActivityGraphNodeHoverClassName,
  FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS,
  factoryGraphEditorEdgeHoverClassName,
} from "./current-activity-graph-hover";

describe("currentActivityGraphNodeHoverClassName", () => {
  it("returns accent hover classes for a neutral node", () => {
    expect(currentActivityGraphNodeHoverClassName({})).toBe(
      CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS,
    );
  });

  it.each([
    ["selected", { selected: true }],
    ["validationError", { validationError: true }],
    ["activeFlow", { activeFlow: true }],
    ["muted", { muted: true }],
  ] as const)("suppresses hover emphasis when %s", (_label, state) => {
    expect(currentActivityGraphNodeHoverClassName(state)).toBeUndefined();
  });
});

describe("currentActivityGraphEdgeHoverClassName", () => {
  it("returns hoverable edge class for a neutral edge", () => {
    expect(currentActivityGraphEdgeHoverClassName({})).toBe(
      CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS,
    );
  });

  it.each([
    ["activeFlow", { activeFlow: true }],
    ["active", { active: true }],
    ["semantic", { semantic: true }],
    ["muted", { muted: true }],
    ["pendingAddition", { pendingAddition: true }],
    ["pendingRemoval", { pendingRemoval: true }],
  ] as const)("suppresses hoverable class when %s", (_label, state) => {
    expect(currentActivityGraphEdgeHoverClassName(state)).toBeUndefined();
  });
});

describe("factoryGraphEditorEdgeHoverClassName", () => {
  it("returns factory editor hoverable class for a neutral edge", () => {
    expect(factoryGraphEditorEdgeHoverClassName({})).toBe(
      FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS,
    );
  });

  it.each([
    ["active", { active: true }],
    ["activeFlow", { activeFlow: true }],
    ["pendingAddition", { pendingAddition: true }],
    ["pendingRemoval", { pendingRemoval: true }],
  ] as const)("suppresses hoverable class when %s", (_label, state) => {
    expect(factoryGraphEditorEdgeHoverClassName(state)).toBeUndefined();
  });
});
