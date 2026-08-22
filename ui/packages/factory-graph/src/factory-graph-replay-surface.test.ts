import { WorkstationKind, WorkstationType } from "@you-agent-factory/client";
import { expect, test } from "vitest";

import { projectFactoryGraphReplayFlow } from "./factory-graph-replay-surface.js";
import type { FactoryGraphWorkstationNode } from "./semantic-workstation-node.js";
import type { FactoryGraphSource } from "./source.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";

test("projects Factory-authored layout, documents, and semantic runtime nodes together", () => {
  const source = {
    factory: {
      layout: {
        edges: [],
        groups: [],
        nodes: [
          {
            id: "worker:alex",
            position: { x: 40, y: 80 },
            size: { height: 100, width: 260 },
          },
          {
            id: "doc:factory/docs/runbook.md",
            position: { x: 640, y: 120 },
            size: { height: 200, width: 360 },
          },
        ],
        viewport: { x: 12, y: 24, zoom: 0.8 },
      },
      name: "Support",
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "UTF8", inline: "# Runbook" },
            targetPath: "factory/docs/runbook.md",
            type: "DOC",
          },
        ],
      },
    },
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: [
          {
            entityId: "alex",
            handles: [],
            id: "worker:alex",
            kind: "worker",
            label: "Alex",
          },
        ],
        ok: true,
        selectedTick: 4,
      },
    },
    selectedTick: 4,
  } satisfies FactoryGraphSource;

  const flow = projectFactoryGraphReplayFlow(
    source,
    "doc:factory/docs/runbook.md",
  );

  expect(flow.nodes).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        height: 100,
        initialHeight: 100,
        initialWidth: 260,
        id: "worker:alex",
        measured: { height: 100, width: 260 },
        position: { x: 40, y: 80 },
        type: "worker",
        width: 260,
      }),
      expect.objectContaining({
        data: expect.objectContaining({
          selectedDoc: true,
          targetPath: "factory/docs/runbook.md",
        }),
        height: 200,
        initialHeight: 200,
        initialWidth: 360,
        id: "doc:factory/docs/runbook.md",
        measured: { height: 200, width: 360 },
        position: { x: 640, y: 120 },
        type: "doc",
        width: 360,
      }),
    ]),
  );
});

test("keeps authored dimensions when replay advances to a later runtime tick", () => {
  const source = {
    factory: {
      layout: {
        nodes: [
          {
            id: "worker:alex",
            position: { x: 40, y: 80 },
            size: { height: 100, width: 260 },
          },
        ],
        schemaVersion: 1,
      },
      name: "Runtime update",
    },
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: [
          {
            entityId: "alex",
            handles: [],
            id: "worker:alex",
            kind: "worker",
            label: "Alex",
          },
        ],
        ok: true,
        selectedTick: 4,
      },
    },
    selectedTick: 4,
  } satisfies FactoryGraphSource;

  const updatedSource = structuredClone(source);
  updatedSource.runtime.activity.selectedTick = 5;
  updatedSource.runtime.load.selectedTick = 5;
  updatedSource.runtime.topology.selectedTick = 5;
  updatedSource.selectedTick = 5;

  const node = projectFactoryGraphReplayFlow(updatedSource).nodes.find(
    (candidate) => candidate.id === "worker:alex",
  );

  expect(node).toMatchObject({
    height: 100,
    initialHeight: 100,
    initialWidth: 260,
    measured: { height: 100, width: 260 },
    width: 260,
  });
});

test("forwards a future worker kind from the replay Factory into the node", () => {
  const source = {
    factory: {
      name: "Future worker kinds",
      workers: [
        {
          name: "next-worker",
          type: "FUTURE_WORKER_KIND" as never,
        },
      ],
    },
    runtime: {
      activity: {
        activeDispatchOverlays: [],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 1,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 1,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: [
          {
            entityId: "next-worker",
            handles: [],
            id: "worker:next-worker",
            kind: "worker",
            label: "next-worker",
          },
        ],
        ok: true,
        selectedTick: 1,
      },
    },
    selectedTick: 1,
  } satisfies FactoryGraphSource;

  const worker = projectFactoryGraphReplayFlow(source).nodes.find(
    (node) => node.id === "worker:next-worker",
  );

  expect(worker?.data).toMatchObject({
    workerType: "FUTURE_WORKER_KIND",
  });
});

test("keeps replay graph identity while selected-tick Work volume changes", () => {
  const sourceForWorkItems = (workIds: string[], selectedTick: number) =>
    ({
      factory: {
        layout: {
          groups: [],
          nodes: [
            {
              id: "workstation:review",
              position: { x: 120, y: 80 },
              size: { height: 160, width: 280 },
            },
          ],
        },
        name: "Work volume",
        workstations: [
          {
            behavior: WorkstationKind.STANDARD,
            id: "review",
            inputs: [],
            name: "Review",
            type: WorkstationType.AGENT_RUN,
            worker: "agent",
          },
        ],
      },
      runtime: {
        activity: {
          activeDispatchOverlays: [
            {
              connectionIds: [],
              dispatchId: "dispatch-review",
              evidence: {
                resources: "unavailable",
                route: "unavailable",
                work: "known",
                worker: "unavailable",
                workstation: "known",
              },
              id: "overlay:dispatch-review",
              startedTick: 4,
              workIds,
              workstationNodeId: "workstation:review",
            },
          ],
          activeWorkstationNodeIds: ["workstation:review"],
          issues: [],
          resourceOccupancy: [],
          selectedTick,
        },
        load: {
          issues: [],
          resourceOccupancy: [],
          selectedTick,
          workStateCounts: [],
        },
        topology: {
          connections: [],
          issues: [],
          nodes: [
            {
              entityId: "review",
              handles: [
                { id: "workstation-input-target", role: "target" },
                { id: "workstation-on-continue-source", role: "source" },
              ],
              id: "workstation:review",
              kind: "workstation",
              label: "Review",
            },
          ],
          ok: true,
          selectedTick,
        },
      },
      selectedTick,
    }) satisfies FactoryGraphSource;

  const initial = projectFactoryGraphReplayFlow(
    sourceForWorkItems(["work-1"], 4),
  );
  const updated = projectFactoryGraphReplayFlow(
    sourceForWorkItems(["work-1", "work-2", "work-3", "work-4"], 5),
  );
  const projectNodeIdentity = (node: (typeof initial.nodes)[number]) => ({
    dimensions: {
      height: node.height,
      initialHeight: node.initialHeight,
      initialWidth: node.initialWidth,
      measured: node.measured,
      width: node.width,
    },
    handles: node.data.handles.map((handle) => handle.id),
    id: node.id,
    position: node.position,
    type: node.type,
  });

  expect(updated.nodes.map(projectNodeIdentity)).toEqual(
    initial.nodes.map(projectNodeIdentity),
  );
  const updatedWorkstation = updated.nodes.find(
    (node): node is FactoryGraphWorkstationNode =>
      node.type === "workstation" && node.id === "workstation:review",
  );
  expect(updatedWorkstation?.data.executions[0]?.work_items).toHaveLength(4);
  expect(updatedWorkstation?.data.summaryOnly).toBe(false);
  expect(
    [1, 3, 4, 25].map((count) => factoryGraphWorkProgressMode(count)),
  ).toEqual(["items", "items", "total", "total"]);
});

test("projects authored workstation semantics and id-based activity onto the graph node", () => {
  const source = {
    factory: {
      name: "Semantic Factory",
      workstations: [
        {
          behavior: WorkstationKind.REPEATER,
          id: "stable-workstation",
          inputs: [],
          name: "Renamed process",
          type: WorkstationType.AGENT_RUN,
          worker: "agent",
        },
      ],
    },
    runtime: {
      activity: {
        activeDispatchOverlays: [
          {
            connectionIds: [],
            dispatchId: "dispatch-1",
            evidence: {
              resources: "unavailable",
              route: "unavailable",
              work: "unavailable",
              worker: "unavailable",
              workstation: "known",
            },
            id: "dispatch:dispatch-1",
            startedTick: 4,
            workstationId: "stable-workstation",
          },
        ],
        activeWorkstationNodeIds: [],
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
      },
      load: {
        issues: [],
        resourceOccupancy: [],
        selectedTick: 4,
        workStateCounts: [],
      },
      topology: {
        connections: [],
        issues: [],
        nodes: [
          {
            entityId: "stable-workstation",
            handles: [],
            id: "workstation:stable-workstation",
            kind: "workstation",
            label: "Renamed process",
          },
        ],
        ok: true,
        selectedTick: 4,
      },
    },
    selectedTick: 4,
  } satisfies FactoryGraphSource;

  const node = projectFactoryGraphReplayFlow(source).nodes[0];

  expect(node).toMatchObject({
    data: {
      active: true,
      activeFlow: true,
      workstationSemantics: {
        controlRole: "NONE",
        runtimeRole: "AGENT",
        runtimeType: WorkstationType.AGENT_RUN,
        schedulingBehavior: WorkstationKind.REPEATER,
      },
    },
    id: "workstation:stable-workstation",
    type: "workstation",
  });
});
