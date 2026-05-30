import { describe, expect, it, vi } from "vitest";

import type { FactoryGraphTopology } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import * as factoryGraphEditorLayout from "../../factory-graph-editor/lib/factory-graph-editor-layout";
import { buildTraceFactoryGraphLayoutPositions } from "./trace-factory-graph-layout";
import { projectTraceDispatchesToFactoryGraph } from "./trace-dispatch-factory-graph";
import { projectTraceRelationsToFactoryGraph } from "./trace-relation-factory-graph";

const EMPTY_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [],
};

describe("buildTraceFactoryGraphLayoutPositions", () => {
  it("returns an empty position map for empty topology without calling layout", async () => {
    const layoutSpy = vi.spyOn(
      factoryGraphEditorLayout,
      "buildFactoryGraphEditorLayout",
    );

    const positions = await buildTraceFactoryGraphLayoutPositions(
      EMPTY_TOPOLOGY,
      new Map(),
    );

    expect(positions.size).toBe(0);
    expect(layoutSpy).not.toHaveBeenCalled();
    layoutSpy.mockRestore();
  });

  it("remaps factory layout node positions to trace React Flow ids", async () => {
    const layoutSpy = vi
      .spyOn(factoryGraphEditorLayout, "buildFactoryGraphEditorLayout")
      .mockResolvedValue({
        height: 200,
        nodes: [
          {
            column: 0,
            height: 196,
            nodeId: "workstation:dispatch-a",
            nodeKind: "workstation",
            row: 0,
            width: 156,
            x: 48,
            y: 72,
          },
        ],
        width: 320,
      });

    const topology: FactoryGraphTopology = {
      edges: [],
      nodes: [
        {
          id: "workstation:dispatch-a",
          key: { kind: "workstation", name: "dispatch-a" },
          kind: "workstation",
          label: "dispatch-a",
        },
      ],
    };

    const positions = await buildTraceFactoryGraphLayoutPositions(
      topology,
      new Map([["workstation:dispatch-a", "dispatch-a"]]),
    );

    expect(layoutSpy).toHaveBeenCalledWith(topology);
    expect(positions.get("dispatch-a")).toEqual({ x: 48, y: 72 });
    expect(positions.has("workstation:dispatch-a")).toBe(false);
    layoutSpy.mockRestore();
  });

  it("omits layout nodes that have no trace React Flow id mapping", async () => {
    const layoutSpy = vi
      .spyOn(factoryGraphEditorLayout, "buildFactoryGraphEditorLayout")
      .mockResolvedValue({
        height: 200,
        nodes: [
          {
            column: 0,
            height: 196,
            nodeId: "workstation:dispatch-a",
            nodeKind: "workstation",
            row: 0,
            width: 156,
            x: 10,
            y: 20,
          },
          {
            column: 1,
            height: 196,
            nodeId: "workstation:dispatch-b",
            nodeKind: "workstation",
            row: 0,
            width: 156,
            x: 220,
            y: 20,
          },
        ],
        width: 400,
      });

    const topology: FactoryGraphTopology = {
      edges: [],
      nodes: [
        {
          id: "workstation:dispatch-a",
          key: { kind: "workstation", name: "dispatch-a" },
          kind: "workstation",
          label: "dispatch-a",
        },
      ],
    };

    const positions = await buildTraceFactoryGraphLayoutPositions(
      topology,
      new Map([["workstation:dispatch-a", "dispatch-a"]]),
    );

    expect(positions.size).toBe(1);
    expect(positions.get("dispatch-a")).toEqual({ x: 10, y: 20 });
    layoutSpy.mockRestore();
  });
});

describe("buildTraceFactoryGraphLayoutPositions dispatch projection", () => {
  it("lays out dispatch projection topology with left-to-right workstation ordering", async () => {
    const projection = projectTraceDispatchesToFactoryGraph([
      {
        dispatch_id: "dispatch-plan",
        duration_millis: 1000,
        end_time: "2026-04-22T18:00:01Z",
        outcome: "ACCEPTED",
        start_time: "2026-04-22T18:00:00Z",
        transition_id: "dispatch-plan",
        workstation_name: "Plan",
      },
      {
        dispatch_id: "dispatch-build",
        duration_millis: 1000,
        end_time: "2026-04-22T18:00:02Z",
        input_items: [
          {
            display_name: "work-reviewed",
            work_id: "work-reviewed",
            work_type_id: "story",
          },
        ],
        outcome: "ACCEPTED",
        previous_chaining_trace_ids: ["trace-plan"],
        start_time: "2026-04-22T18:00:01Z",
        transition_id: "dispatch-build",
        workstation_name: "Build",
      },
    ]);

    const positions = await buildTraceFactoryGraphLayoutPositions(
      projection.topology,
      projection.dispatchIdByNodeId,
    );

    expect(positions.size).toBe(2);
    expect(positions.get("dispatch-plan")?.x).toBeLessThan(
      positions.get("dispatch-build")?.x ?? Number.POSITIVE_INFINITY,
    );
  });
});

describe("buildTraceFactoryGraphLayoutPositions relation projection", () => {
  it("lays out relation projection topology with left-to-right endpoint ordering", async () => {
    const projection = projectTraceRelationsToFactoryGraph([
      {
        request_id: "request-parent-child",
        required_state: "DONE",
        source_work_id: "work-plan",
        source_work_name: "Plan story",
        target_work_id: "work-implement",
        target_work_name: "Implement story",
        type: "PARENT_CHILD",
      },
      {
        request_id: "request-retry",
        required_state: "FAILED",
        source_work_id: "work-implement",
        source_work_name: "Implement story",
        target_work_id: "work-repair",
        target_work_name: "Repair story",
        type: "RETRY",
      },
    ]);

    const positions = await buildTraceFactoryGraphLayoutPositions(
      projection.topology,
      projection.endpointKeyByNodeId,
    );

    expect(positions.size).toBe(3);
    expect(positions.get("work-plan")?.x).toBeLessThan(
      positions.get("work-implement")?.x ?? Number.POSITIVE_INFINITY,
    );
    expect(positions.get("work-implement")?.x).toBeLessThan(
      positions.get("work-repair")?.x ?? Number.POSITIVE_INFINITY,
    );
  });
});
