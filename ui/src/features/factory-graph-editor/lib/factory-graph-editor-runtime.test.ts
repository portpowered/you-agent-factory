import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { buildFactoryGraphWorkerStatusMap } from "./factory-graph-editor-runtime";
import type { CanonicalFactoryDefinition } from "./factory-graph-draft-types";

const factoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    { model: "gpt-5", name: "writer", type: "MODEL_WORKER" },
    { model: "gpt-5", name: "reviewer", type: "MODEL_WORKER" },
    { model: "gpt-5", name: "stalled", type: "MODEL_WORKER" },
  ],
  workstations: [
    {
      inputs: [{ state: "queued", workType: "story" }],
      name: "draft",
      outputs: [{ state: "review", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
    {
      inputs: [{ state: "review", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "reviewer",
    },
    {
      inputs: [{ state: "review", workType: "story" }],
      name: "fallback",
      outputs: [{ state: "done", workType: "story" }],
      type: "MODEL_WORKSTATION",
      worker: "stalled",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
};

function dashboardSnapshot(): DashboardSnapshot {
  return {
    factory_state: "running",
    tick_count: 12,
    topology: {
      edges: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    runtime: {
      active_dispatch_ids: ["dispatch-draft"],
      active_executions_by_dispatch_id: {
        "dispatch-draft": {
          dispatch_id: "dispatch-draft",
          started_at: "2026-05-19T01:10:00Z",
          transition_id: "draft-transition",
          workstation_name: "draft",
          workstation_node_id: "draft-node",
        },
      },
      active_throttle_pauses: [
        {
          affected_worker_types: ["stalled"],
          lane_id: "provider:codex",
          model: "gpt-5",
          paused_until: "2026-05-19T01:15:00Z",
          provider: "OPENAI",
          recover_at: "2026-05-19T01:16:00Z",
        },
      ],
      active_workstation_node_ids: ["draft-node"],
      in_flight_dispatch_count: 1,
      session: {
        completed_count: 0,
        dispatched_count: 1,
        failed_count: 0,
        has_data: true,
      },
      workstation_requests_by_dispatch_id: {
        "dispatch-review": {
          counts: {
            dispatched_count: 1,
            errored_count: 1,
            responded_count: 1,
          },
          dispatch_id: "dispatch-review",
          request: {},
          response: {
            failure_message: "Provider request failed.",
            outcome: "FAILED",
          },
          transition_id: "review-transition",
          workstation_name: "review",
          workstation_node_id: "review-node",
          work_items: [],
        },
      },
    },
    uptime_seconds: 240,
  };
}

describe("factory graph editor runtime", () => {
  it("derives active, errored, unavailable, and idle worker states from runtime data", () => {
    const statuses = buildFactoryGraphWorkerStatusMap({
      factoryDefinition,
      snapshot: dashboardSnapshot(),
    });

    expect(statuses.get("writer")).toBe("active");
    expect(statuses.get("reviewer")).toBe("errored");
    expect(statuses.get("stalled")).toBe("unavailable");
  });

  it("prefers active over older error signals for the same worker", () => {
    const snapshot = dashboardSnapshot();
    snapshot.runtime.workstation_requests_by_dispatch_id = {
      ...snapshot.runtime.workstation_requests_by_dispatch_id,
      "dispatch-draft": {
        counts: {
          dispatched_count: 1,
          errored_count: 1,
          responded_count: 1,
        },
        dispatch_id: "dispatch-draft",
        request: {},
        response: {
          failure_message: "Transient failure.",
          outcome: "FAILED",
        },
        transition_id: "draft-transition",
        workstation_name: "draft",
        workstation_node_id: "draft-node",
        work_items: [],
      },
    };

    const statuses = buildFactoryGraphWorkerStatusMap({
      factoryDefinition,
      snapshot,
    });

    expect(statuses.get("writer")).toBe("active");
  });
});
