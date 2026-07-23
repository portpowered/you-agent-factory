import { describe, expect, it } from "vitest";

import {
  currentActivityGraphEdgeHoverClassName,
  currentActivityGraphNodeHoverClassName,
  factoryGraphEditorEdgeHoverClassName,
} from "./current-activity-graph-hover";

const CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS =
  "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-warning-container hover:opacity-100 hover:shadow-af-accent-chip";
const CURRENT_ACTIVITY_GRAPH_NODE_PRIMARY_HOVER_CLASS =
  "transition-[background-color,border-color,box-shadow,opacity] hover:border-primary hover:bg-primary-container hover:opacity-100 hover:shadow-af-accent-chip";
const CURRENT_ACTIVITY_GRAPH_EDGE_HOVER_CLASS = "agent-flow-edge--hoverable";
const FACTORY_GRAPH_EDITOR_EDGE_HOVER_CLASS =
  "agent-factory-editor-edge--hoverable";

describe("currentActivityGraphNodeHoverClassName", () => {
  it("returns accent hover classes for a neutral node", () => {
    expect(currentActivityGraphNodeHoverClassName({})).toBe(
      CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS,
    );
  });

  it("returns primary surface hover classes when requested", () => {
    expect(currentActivityGraphNodeHoverClassName({}, "primary")).toBe(
      CURRENT_ACTIVITY_GRAPH_NODE_PRIMARY_HOVER_CLASS,
    );
  });

  it.each([
    ["activeFlow", { activeFlow: true }],
    ["muted", { muted: true }],
  ] as const)(
    "keeps hover emphasis when graph context is %s",
    (_label, state) => {
      expect(currentActivityGraphNodeHoverClassName(state)).toBe(
        CURRENT_ACTIVITY_GRAPH_NODE_HOVER_CLASS,
      );
    },
  );

  it.each([
    ["selected", { selected: true }],
    ["validationError", { validationError: true }],
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
