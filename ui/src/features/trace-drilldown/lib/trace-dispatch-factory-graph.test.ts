import { describe, expect, it } from "vitest";
import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { projectTraceDispatchesToFactoryGraph } from "./trace-dispatch-factory-graph";

function buildWorkItem(
  workID: string,
  overrides: Partial<DashboardWorkItemRef> = {},
): DashboardWorkItemRef {
  return {
    display_name: workID,
    work_id: workID,
    work_type_id: "story",
    ...overrides,
  };
}

function buildDispatch(
  dispatchID: string,
  overrides: Partial<DashboardTraceDispatch> = {},
): DashboardTraceDispatch {
  return {
    dispatch_id: dispatchID,
    duration_millis: 1000,
    end_time: "2026-04-22T18:00:01Z",
    outcome: "ACCEPTED",
    start_time: "2026-04-22T18:00:00Z",
    transition_id: dispatchID,
    workstation_name: dispatchID,
    ...overrides,
  };
}

function factoryEdgePairs(
  projection: ReturnType<typeof projectTraceDispatchesToFactoryGraph>,
): string[] {
  return projection.topology.edges
    .map((edge) => `${edge.sourceId}->${edge.targetId}`)
    .sort();
}

function dispatchEdgePairs(
  projection: ReturnType<typeof projectTraceDispatchesToFactoryGraph>,
): string[] {
  return projection.topology.edges
    .map((edge) => {
      const sourceDispatchId = projection.dispatchIdByNodeId.get(edge.sourceId);
      const targetDispatchId = projection.dispatchIdByNodeId.get(edge.targetId);
      return `${sourceDispatchId}->${targetDispatchId}`;
    })
    .sort();
}

describe("projectTraceDispatchesToFactoryGraph explicit lineage", () => {
  it("prefers explicit predecessor chains and preserves fan-in edges", () => {
    const projection = projectTraceDispatchesToFactoryGraph([
      buildDispatch("dispatch-plan", {
        current_chaining_trace_id: "trace-plan-chain",
        output_items: [
          buildWorkItem("work-reviewed", {
            current_chaining_trace_id: "trace-plan-chain",
          }),
        ],
      }),
      buildDispatch("dispatch-research", {
        current_chaining_trace_id: "trace-research-chain",
        output_items: [
          buildWorkItem("work-context", {
            current_chaining_trace_id: "trace-research-chain",
          }),
        ],
      }),
      buildDispatch("dispatch-implement", {
        input_items: [buildWorkItem("work-reviewed")],
        previous_chaining_trace_ids: [
          "trace-plan-chain",
          "trace-research-chain",
        ],
      }),
    ]);

    expect(
      dispatchEdgePairs(projection).filter((edge) =>
        edge.endsWith("->dispatch-implement"),
      ),
    ).toEqual([
      "dispatch-plan->dispatch-implement",
      "dispatch-research->dispatch-implement",
    ]);
    expect(
      factoryEdgePairs(projection).filter((edge) =>
        edge.endsWith("->workstation:dispatch-implement"),
      ),
    ).toEqual([
      "workstation:dispatch-plan->workstation:dispatch-implement",
      "workstation:dispatch-research->workstation:dispatch-implement",
    ]);
    expect(
      projection.topology.edges.every(
        (edge) => edge.kind === "workstation-on-continue",
      ),
    ).toBe(true);
  });
});

describe("projectTraceDispatchesToFactoryGraph fallback lineage", () => {
  it("falls back to output-to-input work lineage when chaining metadata is absent", () => {
    const projection = projectTraceDispatchesToFactoryGraph([
      buildDispatch("dispatch-plan", {
        output_items: [buildWorkItem("work-reviewed")],
      }),
      buildDispatch("dispatch-implement", {
        input_items: [buildWorkItem("work-reviewed")],
      }),
    ]);

    expect(dispatchEdgePairs(projection)).toEqual([
      "dispatch-plan->dispatch-implement",
    ]);
  });

  it("falls back to sequential ordering when no explicit or work-item lineage is available", () => {
    const projection = projectTraceDispatchesToFactoryGraph([
      buildDispatch("dispatch-plan"),
      buildDispatch("dispatch-review"),
      buildDispatch("dispatch-implement"),
    ]);

    expect(dispatchEdgePairs(projection)).toEqual([
      "dispatch-plan->dispatch-review",
      "dispatch-review->dispatch-implement",
    ]);
  });
});

describe("projectTraceDispatchesToFactoryGraph workstation node ids", () => {
  it("uses workstation names when unique and falls back to dispatch ids for repeated names", () => {
    const projection = projectTraceDispatchesToFactoryGraph([
      buildDispatch("8a56a3ce-6277-41d8-9bc8-840aa10a8d74", {
        transition_id: "plan",
        workstation_name: "plan",
      }),
      buildDispatch("145638a2-67c9-4f2a-8a7d-6297ebcd7a19", {
        transition_id: "setup-workspace",
        workstation_name: "setup-workspace",
      }),
      buildDispatch("a5399deb-dffe-4a6d-9b4f-0310aa988bf2", {
        transition_id: "process",
        workstation_name: "process",
      }),
      buildDispatch("534f91c4-4e83-4310-b211-dbb3ee3cabd1", {
        transition_id: "process",
        workstation_name: "process",
      }),
      buildDispatch("be0ca2a8-c4f7-42c2-8bd3-a54c0bd9de25", {
        transition_id: "process",
        workstation_name: "process",
      }),
      buildDispatch("74d8f3b3-d91b-4bcc-927d-b2643e71bc8a", {
        transition_id: "process",
        workstation_name: "process",
      }),
      buildDispatch("82d4be6a-68c3-4c94-ad3b-53fd53326015", {
        transition_id: "process",
        workstation_name: "process",
      }),
    ]);

    expect(projection.topology.nodes.map((node) => node.id)).toEqual([
      "workstation:plan",
      "workstation:setup-workspace",
      "workstation:process",
      "workstation:534f91c4-4e83-4310-b211-dbb3ee3cabd1",
      "workstation:be0ca2a8-c4f7-42c2-8bd3-a54c0bd9de25",
      "workstation:74d8f3b3-d91b-4bcc-927d-b2643e71bc8a",
      "workstation:82d4be6a-68c3-4c94-ad3b-53fd53326015",
    ]);
    expect([...projection.nodeIdByDispatchId.values()]).toEqual([
      "workstation:plan",
      "workstation:setup-workspace",
      "workstation:process",
      "workstation:534f91c4-4e83-4310-b211-dbb3ee3cabd1",
      "workstation:be0ca2a8-c4f7-42c2-8bd3-a54c0bd9de25",
      "workstation:74d8f3b3-d91b-4bcc-927d-b2643e71bc8a",
      "workstation:82d4be6a-68c3-4c94-ad3b-53fd53326015",
    ]);
    expect(projection.topology.nodes.every((node) => node.kind === "workstation"))
      .toBe(true);
  });
});

describe("projectTraceDispatchesToFactoryGraph overlays", () => {
  it("keeps canonical factory node labels while exposing trace overlay metadata", () => {
    const projection = projectTraceDispatchesToFactoryGraph([
      buildDispatch("dispatch-plan", {
        input_items: [buildWorkItem("work-input")],
        outcome: "ACCEPTED",
        output_items: [buildWorkItem("work-output")],
        workstation_name: "plan",
      }),
    ]);
    const node = projection.topology.nodes[0];

    expect(node.label).toBe("plan");
    expect(node.kind).toBe("workstation");
    expect(projection.overlaysByNodeId.get(node.id)).toEqual({
      dispatchId: "dispatch-plan",
      displayLabel: "plan",
      inputSummary: "(story):work-input",
      outcome: "ACCEPTED",
      outputSummary: "(story):work-output",
    });
  });
});
