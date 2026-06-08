import { describe, expect, it } from "vitest";

import type { CurrentActivityNode } from "../../../flowchart/public";
import { mergeBaseNodesWithPresentationPositions } from "./merge-base-nodes-with-presentation-positions";

function node(
  id: string,
  position: { x: number; y: number },
): CurrentActivityNode {
  return {
    data: {},
    id,
    position,
    type: "currentActivity",
  } as CurrentActivityNode;
}

describe("mergeBaseNodesWithPresentationPositions", () => {
  it("preserves local presentation positions while canonical layout is unchanged", () => {
    const previousBaseNodes = [node("workstation:draft", { x: 10, y: 20 })];
    const baseNodes = [node("workstation:draft", { x: 10, y: 20 })];
    const presentationNodes = [node("workstation:draft", { x: 42, y: 84 })];

    expect(
      mergeBaseNodesWithPresentationPositions(
        baseNodes,
        previousBaseNodes,
        presentationNodes,
      ),
    ).toEqual([node("workstation:draft", { x: 42, y: 84 })]);
  });

  it("replaces stale presentation positions when canonical layout changes", () => {
    const previousBaseNodes = [node("workstation:draft", { x: 42, y: 84 })];
    const baseNodes = [node("workstation:draft", { x: 10, y: 20 })];
    const presentationNodes = [node("workstation:draft", { x: 42, y: 84 })];

    expect(
      mergeBaseNodesWithPresentationPositions(
        baseNodes,
        previousBaseNodes,
        presentationNodes,
      ),
    ).toEqual([node("workstation:draft", { x: 10, y: 20 })]);
  });

  it("drops removed nodes even when presentation state still contains them", () => {
    const previousBaseNodes = [
      node("worker:writer", { x: 10, y: 20 }),
      node("worker:spare", { x: 30, y: 40 }),
    ];
    const baseNodes = [node("worker:writer", { x: 10, y: 20 })];
    const presentationNodes = [
      node("worker:writer", { x: 42, y: 84 }),
      node("worker:spare", { x: 99, y: 99 }),
    ];

    expect(
      mergeBaseNodesWithPresentationPositions(
        baseNodes,
        previousBaseNodes,
        presentationNodes,
      ),
    ).toEqual([node("worker:writer", { x: 42, y: 84 })]);
  });

  it("uses canonical positions for newly introduced nodes", () => {
    const previousBaseNodes = [node("workstation:draft", { x: 10, y: 20 })];
    const baseNodes = [
      node("workstation:draft", { x: 10, y: 20 }),
      node("workstation:review", { x: 30, y: 40 }),
    ];
    const presentationNodes = [node("workstation:draft", { x: 42, y: 84 })];

    expect(
      mergeBaseNodesWithPresentationPositions(
        baseNodes,
        previousBaseNodes,
        presentationNodes,
      ),
    ).toEqual([
      node("workstation:draft", { x: 42, y: 84 }),
      node("workstation:review", { x: 30, y: 40 }),
    ]);
  });
});
