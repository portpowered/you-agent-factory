import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../../api/dashboard/types";

const uiRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "..",
  "..",
  "..",
);
const repoRoot = path.resolve(uiRoot, "..");

export const localAgentCliRuntimeBatchPath = path.join(
  repoRoot,
  "tests",
  "functional",
  "smoke",
  "testdata",
  "factory-batch-local-agent-cli-runtime.json",
);

export const selectedWorkItem: DashboardWorkItemRef = {
  display_name: "Active Story",
  state: "in_progress",
  trace_id: "trace-active-story",
  work_id: "work-active-story",
  work_type_id: "story",
};

export function snapshotFixture(): DashboardSnapshot & {
  relationsByWorkID: Record<string, Array<Record<string, string>>>;
} {
  return {
    factory_state: "RUNNING",
    relationsByWorkID: {
      "work-active-story": [
        {
          source_work_id: "work-active-story",
          sourceWorkName: "Active Story",
          targetWorkId: "work-dependency-story",
          targetWorkName: "Dependency Story",
          type: "DEPENDS_ON",
          requiredState: "ready",
        },
        {
          source_work_id: "work-active-story",
          sourceWorkName: "Active Story",
          targetWorkId: "work-parent-story",
          targetWorkName: "Parent Story",
          type: "PARENT_CHILD",
        },
      ],
      "work-blocked-story": [
        {
          source_work_id: "work-blocked-story",
          sourceWorkName: "Blocked Story",
          targetWorkId: "work-active-story",
          targetWorkName: "Active Story",
          type: "DEPENDS_ON",
          requiredState: "approved",
        },
      ],
      "work-child-story": [
        {
          source_work_id: "work-child-story",
          sourceWorkName: "Child Story",
          targetWorkId: "work-active-story",
          targetWorkName: "Active Story",
          type: "PARENT_CHILD",
        },
      ],
      "work-grandchild-story": [
        {
          source_work_id: "work-child-story",
          sourceWorkName: "Child Story",
          targetWorkId: "work-grandchild-story",
          targetWorkName: "Grandchild Story",
          type: "PARENT_CHILD",
        },
      ],
    },
    runtime: {
      active_executions_by_dispatch_id: {
        "dispatch-active-story": {
          dispatch_id: "dispatch-active-story",
          started_at: "2026-05-26T10:00:00Z",
          transition_id: "transition-story",
          work_items: [
            selectedWorkItem,
            {
              display_name: "Dependency Story",
              state: "ready",
              trace_id: "trace-dependency-story",
              work_id: "work-dependency-story",
              work_type_id: "story",
            },
            {
              display_name: "Parent Story",
              state: "queued",
              trace_id: "trace-parent-story",
              work_id: "work-parent-story",
              work_type_id: "epic",
            },
            {
              display_name: "Blocked Story",
              state: "blocked",
              trace_id: "trace-blocked-story",
              work_id: "work-blocked-story",
              work_type_id: "story",
            },
            {
              display_name: "Child Story",
              state: "queued",
              trace_id: "trace-child-story",
              work_id: "work-child-story",
              work_type_id: "task",
            },
            {
              display_name: "Grandchild Story",
              state: "queued",
              trace_id: "trace-grandchild-story",
              work_id: "work-grandchild-story",
              work_type_id: "task",
            },
          ],
          workstation_node_id: "transition-story",
        },
      },
      in_flight_dispatch_count: 1,
      session: {
        completed_count: 0,
        dispatched_count: 1,
        failed_count: 0,
        has_data: true,
      },
    },
    tick_count: 4,
    topology: {
      edges: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: 42,
  };
}

interface BatchWork {
  name: string;
  workTypeName: string;
  payload?: { title?: string };
}

interface BatchRelation {
  type: string;
  sourceWorkName: string;
  targetWorkName: string;
  requiredState?: string;
}

interface BatchDocument {
  relations: BatchRelation[];
  works: BatchWork[];
}

function workIDForName(workName: string): string {
  return `work-${workName}`;
}

function displayNameForWork(work: BatchWork): string {
  return work.payload?.title?.trim() || work.name;
}

export function loadLocalAgentCliRuntimeBatch(): BatchDocument {
  const raw = readFileSync(localAgentCliRuntimeBatchPath, "utf8");
  return JSON.parse(raw) as BatchDocument;
}

export function localAgentCliRuntimeLoopbackWorkItem(): DashboardWorkItemRef {
  const batch = loadLocalAgentCliRuntimeBatch();
  const loopback = batch.works.find(
    (work) => work.name === "local-agent-cli-runtime-loopback",
  );
  if (!loopback) {
    throw new Error(
      "Expected loopback work item in local agent CLI runtime batch.",
    );
  }

  return {
    display_name: displayNameForWork(loopback),
    state: "queued",
    trace_id: "trace-local-agent-cli-runtime-loopback",
    work_id: workIDForName(loopback.name),
    work_type_id: loopback.workTypeName,
  };
}

export function localAgentCliRuntimeBatchSnapshot(): DashboardSnapshot & {
  relationsByWorkID: Record<string, Array<Record<string, string>>>;
} {
  const batch = loadLocalAgentCliRuntimeBatch();
  const workItems: DashboardWorkItemRef[] = batch.works.map((work) => ({
    display_name: displayNameForWork(work),
    state: "queued",
    trace_id: `trace-${work.name}`,
    work_id: workIDForName(work.name),
    work_type_id: work.workTypeName,
  }));

  const relationsByWorkID: Record<string, Array<Record<string, string>>> = {};
  for (const relation of batch.relations) {
    const sourceWorkID = workIDForName(relation.sourceWorkName);
    const targetWorkID = workIDForName(relation.targetWorkName);
    const entry: Record<string, string> = {
      source_work_id: sourceWorkID,
      sourceWorkName: relation.sourceWorkName,
      targetWorkId: targetWorkID,
      targetWorkName: relation.targetWorkName,
      type: relation.type,
    };
    if (relation.requiredState) {
      entry.requiredState = relation.requiredState;
    }
    relationsByWorkID[sourceWorkID] = [
      ...(relationsByWorkID[sourceWorkID] ?? []),
      entry,
    ];
    if (!relationsByWorkID[targetWorkID]) {
      relationsByWorkID[targetWorkID] = [];
    }
  }

  return {
    factory_state: "RUNNING",
    relationsByWorkID,
    runtime: {
      active_executions_by_dispatch_id: {
        "dispatch-local-agent-cli-runtime": {
          dispatch_id: "dispatch-local-agent-cli-runtime",
          started_at: "2026-06-16T12:00:00Z",
          transition_id: "transition-local-agent-cli-runtime",
          work_items: workItems,
          workstation_node_id: "transition-local-agent-cli-runtime",
        },
      },
      in_flight_dispatch_count: 1,
      session: {
        completed_count: 0,
        dispatched_count: 1,
        failed_count: 0,
        has_data: true,
      },
    },
    tick_count: 1,
    topology: {
      edges: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: 1,
  };
}
