import { WorkstationKind, WorkstationType } from "@you-agent-factory/client";
import { expect, test } from "vitest";

import { projectFactoryGraphReplayFlow } from "./factory-graph-replay-surface.js";
import type { FactoryGraphSource } from "./source.js";

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
