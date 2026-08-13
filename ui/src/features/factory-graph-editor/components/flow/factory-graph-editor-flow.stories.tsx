import { Background, Controls, ReactFlow } from "@xyflow/react";
import {
  FactoryGraphNodeShell,
  type FactoryGraphNodeShellProps,
  type FactoryGraphVisualStateInput,
  GraphSemanticIcon,
} from "@you-agent-factory/factory-graph";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";

import "../../../../styles.css";
import { applyDocumentColorPalette } from "../../../../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../../../../theme/color-palette";
import { baseFactoryDefinition } from "../../lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../../lib/draft/factory-graph-draft-graph";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
  FactoryWorkstation,
} from "../../lib/draft/factory-graph-draft-types";
import type { FactoryGraphConnectionEndpoint } from "../../lib/editor/factory-graph-editor-connections";
import { getFactoryGraphEditorMessages } from "../../messages/editor";
import { FactoryGraphEditorWorkStatePhaseLegend } from "../chrome/factory-graph-editor-work-state-phase-legend";
import { FactoryGraphEditorVisibilityPanel } from "../controls/factory-graph-editor-controls";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "../flow/factory-graph-editor-flow";

const PENDING_REMOVAL_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "worker-assignment:worker:writer->workstation:review",
      kind: "worker-assignment",
      source: { kind: "worker", name: "writer" },
      sourceId: "worker:writer",
      target: { kind: "workstation", name: "review" },
      targetId: "workstation:review",
    },
    {
      id: "workstation-output:review->story:complete",
      kind: "workstation-output",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "complete",
        workTypeName: "story",
      },
      targetId: "work-state:story:complete",
    },
  ],
  nodes: [
    {
      id: "worker:writer",
      key: { kind: "worker", name: "writer" },
      kind: "worker",
      label: "writer",
    },
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
    {
      id: "work-state:story:complete",
      key: {
        kind: "work-state",
        stateName: "complete",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:complete",
    },
  ],
};

function PendingRemovalStory() {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: false,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set([
      "worker-assignment:worker:writer->workstation:review",
      "workstation-output:review->story:complete",
    ]),
    pendingRemovalNodeIds: new Set(["workstation:review"]),
    topology: PENDING_REMOVAL_TOPOLOGY,
  });

  return (
    <div className="h-[520px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4">
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

const CONNECTABLE_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

function ConnectionAnchorsStory() {
  const [pendingConnectionSource, setPendingConnectionSource] =
    useState<FactoryGraphConnectionEndpoint | null>(null);
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: true,
    onConnectionAnchorClick: (endpoint) => {
      setPendingConnectionSource((currentSource) =>
        currentSource &&
        currentSource.nodeId === endpoint.nodeId &&
        currentSource.anchorId === endpoint.anchorId
          ? null
          : endpoint,
      );
    },
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: CONNECTABLE_TOPOLOGY,
  });

  return (
    <div className="h-[520px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4">
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

const PENDING_EDGE_CHANGES_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "workstation-output:workstation:review->work-state:story:done",
      kind: "workstation-output",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      targetId: "work-state:story:done",
    },
    {
      id: "workstation-on-failure:workstation:review->work-state:story:queued",
      kind: "workstation-on-failure",
      source: { kind: "workstation", name: "review" },
      sourceId: "workstation:review",
      target: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      targetId: "work-state:story:queued",
    },
  ],
  nodes: [
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
    {
      id: "work-state:story:done",
      key: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:done",
    },
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
  ],
};

function PendingEdgeChangesStory() {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: false,
    pendingAdditionEdgeIds: new Set([
      "workstation-on-failure:workstation:review->work-state:story:queued",
    ]),
    pendingConnectionSource: null,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set([
      "workstation-output:workstation:review->work-state:story:done",
    ]),
    pendingRemovalNodeIds: new Set<string>(),
    topology: PENDING_EDGE_CHANGES_TOPOLOGY,
  });

  return (
    <div className="h-[520px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4">
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

const WORKER_RESOURCE_TOPOLOGY: FactoryGraphTopology = {
  edges: [
    {
      id: "worker-assignment:worker:writer->workstation:draft",
      kind: "worker-assignment",
      source: { kind: "worker", name: "writer" },
      sourceId: "worker:writer",
      target: { kind: "workstation", name: "draft" },
      targetId: "workstation:draft",
    },
    {
      id: "worker-assignment:worker:reviewer->workstation:review",
      kind: "worker-assignment",
      source: { kind: "worker", name: "reviewer" },
      sourceId: "worker:reviewer",
      target: { kind: "workstation", name: "review" },
      targetId: "workstation:review",
    },
    {
      id: "worker-resource:resource:gpu->worker:writer",
      kind: "worker-resource",
      source: { kind: "resource", name: "gpu" },
      sourceId: "resource:gpu",
      target: { kind: "worker", name: "writer" },
      targetId: "worker:writer",
    },
    {
      id: "workstation-resource:resource:gpu->workstation:draft",
      kind: "workstation-resource",
      source: { kind: "resource", name: "gpu" },
      sourceId: "resource:gpu",
      target: { kind: "workstation", name: "draft" },
      targetId: "workstation:draft",
    },
  ],
  nodes: [
    {
      id: "resource:gpu",
      key: { kind: "resource", name: "gpu" },
      kind: "resource",
      label: "gpu",
    },
    {
      id: "worker:reviewer",
      key: { kind: "worker", name: "reviewer" },
      kind: "worker",
      label: "reviewer",
    },
    {
      id: "worker:stalled",
      key: { kind: "worker", name: "stalled" },
      kind: "worker",
      label: "stalled",
    },
    {
      id: "worker:writer",
      key: { kind: "worker", name: "writer" },
      kind: "worker",
      label: "writer",
    },
    {
      id: "workstation:draft",
      key: { kind: "workstation", name: "draft" },
      kind: "workstation",
      label: "draft",
    },
    {
      id: "workstation:review",
      key: { kind: "workstation", name: "review" },
      kind: "workstation",
      label: "review",
    },
  ],
};

function WorkerResourceDensityStory() {
  const [selectedPreset, setSelectedPreset] = useState<
    "all" | "workflow" | "execution" | "infrastructure"
  >("all");
  const visibleNodes = WORKER_RESOURCE_TOPOLOGY.nodes.filter((node) => {
    if (selectedPreset === "all") {
      return true;
    }
    if (selectedPreset === "workflow") {
      return (
        node.kind === "workstation" ||
        node.kind === "work-type" ||
        node.kind === "work-state"
      );
    }
    if (selectedPreset === "execution") {
      return node.kind === "workstation" || node.kind === "work-state";
    }
    return (
      node.kind === "resource" ||
      node.kind === "worker" ||
      node.kind === "workstation"
    );
  });
  const visibleNodeIds = new Set(visibleNodes.map((node) => node.id));
  const visibleTopology = {
    edges: WORKER_RESOURCE_TOPOLOGY.edges.filter((edge) => {
      if (
        !visibleNodeIds.has(edge.sourceId) ||
        !visibleNodeIds.has(edge.targetId)
      ) {
        return false;
      }
      if (selectedPreset === "workflow") {
        return (
          edge.kind === "work-type-state" ||
          edge.kind === "workstation-input" ||
          edge.kind === "workstation-output"
        );
      }
      if (selectedPreset === "execution") {
        return (
          edge.kind === "work-type-state" ||
          edge.kind === "workstation-input" ||
          edge.kind === "workstation-output" ||
          edge.kind === "workstation-on-continue" ||
          edge.kind === "workstation-on-failure" ||
          edge.kind === "workstation-on-rejection"
        );
      }
      if (selectedPreset === "infrastructure") {
        return (
          edge.kind === "worker-assignment" ||
          edge.kind === "worker-resource" ||
          edge.kind === "workstation-resource"
        );
      }
      return true;
    }),
    nodes: visibleNodes,
  };
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: false,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: visibleTopology,
    workerStatusByName: new Map([
      ["writer", "active"],
      ["reviewer", "errored"],
      ["stalled", "unavailable"],
    ]),
  });

  return (
    <div className="relative h-[560px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4">
      <FactoryGraphEditorVisibilityPanel
        onSelectPreset={setSelectedPreset}
        options={[
          {
            key: "all",
            label: "All",
            selected: selectedPreset === "all",
          },
          {
            key: "workflow",
            label: "Workflow",
            selected: selectedPreset === "workflow",
          },
          {
            key: "execution",
            label: "Execution",
            selected: selectedPreset === "execution",
          },
          {
            key: "infrastructure",
            label: "Infrastructure",
            selected: selectedPreset === "infrastructure",
          },
        ]}
        visible={true}
      />
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

const PROGRESS_OUTCOME_ROUTE_TOPOLOGY: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "workstation:draft",
      key: { kind: "workstation", name: "draft" },
      kind: "workstation",
      label: "draft",
    },
    {
      id: "work-state:story:done",
      key: {
        kind: "work-state",
        stateName: "done",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:done",
    },
    {
      id: "work-state:story:queued",
      key: {
        kind: "work-state",
        stateName: "queued",
        workTypeName: "story",
      },
      kind: "work-state",
      label: "story:queued",
    },
  ],
};

const standardProcessorWithoutStopWords: FactoryWorkstation = {
  ...baseFactoryDefinition.workstations[0],
  behavior: "STANDARD",
  stopWords: undefined,
};

const standardProcessorWithStopWords: FactoryWorkstation = {
  ...baseFactoryDefinition.workstations[0],
  behavior: "STANDARD",
  stopWords: ["DONE"],
};

const factoryWithWorkerStopToken = {
  ...baseFactoryDefinition,
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "rejected", type: "FAILED" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workers: [
    {
      name: "processor",
      stopToken: "<COMPLETE>",
      type: "MODEL_WORKER",
    },
  ],
  workstations: [
    {
      ...baseFactoryDefinition.workstations[0],
      behavior: "STANDARD",
      onContinue: [{ state: "queued", workType: "story" }],
      onFailure: [{ state: "rejected", workType: "story" }],
      onRejection: [{ state: "rejected", workType: "story" }],
      stopWords: undefined,
      worker: "processor",
    },
  ],
} satisfies CanonicalFactoryDefinition;

const logicalMoveWorkstation: FactoryWorkstation = {
  body: "Move work downstream.",
  inputs: [
    {
      state: "queued",
      workType: "story",
    },
  ],
  name: "router",
  outputs: [
    {
      state: "done",
      workType: "story",
    },
  ],
  resources: [
    {
      capacity: 2,
      name: "gpu",
    },
  ],
  type: "LOGICAL_MOVE",
  worker: "",
};

const logicalMoveComparisonTopology: FactoryGraphTopology = {
  ...PROGRESS_OUTCOME_ROUTE_TOPOLOGY,
  nodes: [
    ...PROGRESS_OUTCOME_ROUTE_TOPOLOGY.nodes,
    {
      id: "workstation:router",
      key: { kind: "workstation", name: "router" },
      kind: "workstation",
      label: "router",
    },
  ],
};

function ProgressOutcomeRoutesStory(input: {
  factoryDefinition?: CanonicalFactoryDefinition;
  topology?: FactoryGraphTopology;
  workstations?: readonly FactoryWorkstation[];
}) {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: true,
    factoryDefinition: input.factoryDefinition,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: input.topology ?? PROGRESS_OUTCOME_ROUTE_TOPOLOGY,
    workstations:
      input.workstations ?? input.factoryDefinition?.workstations ?? [],
  });

  return (
    <div className="h-[520px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4">
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

async function expectLogicalMoveConnectHandles(
  canvas: ReturnType<typeof within>,
) {
  await expect(
    canvas.getByRole("button", { name: "Connect: router Success" }),
  ).toBeVisible();
  await expect(
    canvas.getByRole("button", { name: "Connect: router Input" }),
  ).toBeVisible();
  await expect(
    canvas.getByRole("button", { name: "Connect: router Resource" }),
  ).toBeVisible();
  await expect(
    canvas.queryByRole("button", { name: "Connect: router Failure" }),
  ).toBeNull();
  await expect(
    canvas.queryByRole("button", { name: "Connect: router Continue" }),
  ).toBeNull();
  await expect(
    canvas.queryByRole("button", { name: "Connect: router Reject" }),
  ).toBeNull();
}

async function expectProgressOutcomeRouteHandles(
  canvas: ReturnType<typeof within>,
  input: { includeContinueAndReject: boolean },
) {
  await expect(
    canvas.getByRole("button", { name: "Connect: draft Success" }),
  ).toBeVisible();
  await expect(
    canvas.getByRole("button", { name: "Connect: draft Failure" }),
  ).toBeVisible();

  if (input.includeContinueAndReject) {
    await expect(
      canvas.getByRole("button", { name: "Connect: draft Continue" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Connect: draft Reject" }),
    ).toBeVisible();
    return;
  }

  await expect(
    canvas.queryByRole("button", { name: "Connect: draft Continue" }),
  ).toBeNull();
  await expect(
    canvas.queryByRole("button", { name: "Connect: draft Reject" }),
  ).toBeNull();
}

async function expectZAxisIncompleteHints(
  canvasElement: HTMLElement,
  input: { expectHints: boolean },
) {
  const hints = canvasElement.querySelectorAll("[data-z-axis-incomplete-hint]");

  if (!input.expectHints) {
    await expect(hints).toHaveLength(0);
    return;
  }

  await expect(hints).toHaveLength(2);
  const hintMessage =
    getFactoryGraphEditorMessages().zAxisIncompleteConnectionHint;
  for (const hint of hints) {
    await expect(hint.getAttribute("aria-label")).toBe(hintMessage);
    await expect(hint.getAttribute("title")).toBe(hintMessage);
  }
  await expect(
    canvasElement.querySelector(
      '[data-z-axis-incomplete-hint="workstation-on-continue-source"]',
    ),
  ).not.toBeNull();
  await expect(
    canvasElement.querySelector(
      '[data-z-axis-incomplete-hint="workstation-on-rejection-source"]',
    ),
  ).not.toBeNull();
}

const lifecycleFactoryDefinition = {
  ...baseFactoryDefinition,
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
} satisfies CanonicalFactoryDefinition;

const LIFECYCLE_PHASE_TOPOLOGY = buildFactoryGraphTopologyFromDefinition(
  lifecycleFactoryDefinition,
);

function WorkStateLifecyclePhasesStory() {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: false,
    factoryDefinition: lifecycleFactoryDefinition,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingAdditionNodeIds: new Set<string>(),
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: LIFECYCLE_PHASE_TOPOLOGY,
  });

  return (
    <div className="relative h-[520px] w-full rounded-[1.5rem] border border-outline bg-surface-container-high p-4">
      <FactoryGraphEditorWorkStatePhaseLegend visible={true} />
      <ReactFlow
        defaultEdgeOptions={{ selectable: false }}
        edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
        edges={flow.edges}
        fitView={true}
        nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
        nodes={flow.nodes}
        nodesDraggable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}

function ReducedMotionPreviewStory() {
  return (
    <div
      className="grid max-w-xl gap-4 rounded-[1.5rem] border border-outline bg-surface-container-high p-6 text-on-surface"
      data-graph-reduced-motion="true"
    >
      <div className="grid gap-1">
        <h2 className="m-0 text-lg font-bold">Reduced motion preview</h2>
        <p className="m-0 text-sm text-on-surface-variant">
          Active-flow emphasis keeps its semantic border and surface while
          motion is disabled.
        </p>
      </div>
      <div
        className="grid min-h-24 place-items-center rounded-lg border border-outline bg-background p-4"
        data-current-activity-flow
      >
        <article
          className="agent-flow-node--active rounded-lg border-2 border-af-success-border bg-warning-container px-4 py-3 shadow-af-success-chip"
          data-testid="reduced-motion-active-node"
        >
          Active flow
        </article>
      </div>
    </div>
  );
}

function RuntimeEmphasisStatesStory() {
  return (
    <div className="grid max-w-4xl gap-4 rounded-[1.5rem] border border-outline bg-surface-container-high p-6 text-on-surface">
      <div className="grid gap-1">
        <h2 className="m-0 text-lg font-bold">Runtime emphasis states</h2>
        <p className="m-0 text-sm text-on-surface-variant">
          Selection, keyboard focus, muted context, and validation remain
          visible on top of lifecycle meaning.
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <RuntimeEmphasisExample
          data-testid="idle-muted-node"
          iconKind="doc"
          label="Idle context"
          nodeType="doc"
          visualState={{ muted: true }}
        />
        <RuntimeEmphasisExample
          data-testid="active-selected-node"
          iconKind="processing"
          label="Active selected"
          nodeType="workType"
          visualState={{
            activeFlow: true,
            focused: true,
            lifecycle: "PROCESSING",
            selected: true,
          }}
        />
        <RuntimeEmphasisExample
          data-testid="failed-validation-node"
          iconKind="failed"
          label="Failed validation"
          nodeType="statePosition"
          visualState={{ lifecycle: "FAILED", validation: true }}
        />
      </div>
    </div>
  );
}

function RuntimeEmphasisExample({
  "data-testid": testId,
  iconKind,
  label,
  nodeType,
  visualState,
}: {
  "data-testid": string;
  iconKind: "doc" | "failed" | "processing";
  label: string;
  nodeType: FactoryGraphNodeShellProps["nodeType"];
  visualState?: Omit<FactoryGraphVisualStateInput, "family">;
}) {
  return (
    <div data-testid={testId}>
      <FactoryGraphNodeShell
        className="min-h-20 justify-center px-3 py-2"
        handles={[]}
        nodeType={nodeType}
        visualState={visualState}
      >
        <div className="flex items-center gap-2">
          <GraphSemanticIcon kind={iconKind} label={label} />
          <span className="font-semibold">{label}</span>
        </div>
      </FactoryGraphNodeShell>
    </div>
  );
}

export default {
  title: "Agent Factory/Dashboard/Factory Graph Editor Flow",
  tags: ["test"],
};

export const WorkStateLifecyclePhases = {
  render: () => <WorkStateLifecyclePhasesStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    const expectPhaseSurface = async (
      label: string,
      surfaceClass: string,
      status: string,
    ) => {
      const node = (await canvas.findByText(label)).closest("article");
      if (!node) {
        throw new Error(`Expected work-state node for ${label}`);
      }
      for (const className of surfaceClass.split(" ")) {
        await expect(node.className).toContain(className);
      }
      await expect(node).toHaveAttribute("data-graph-visual-status", status);
      await expect(node).toHaveAttribute("data-graph-visual-surface", status);
    };

    await expectPhaseSurface(
      "story:queued",
      "border-info-border bg-info-container",
      "waiting",
    );
    await expectPhaseSurface(
      "story:review",
      "border-af-success-border bg-warning-container",
      "active",
    );
    await expectPhaseSurface(
      "story:done",
      "border-af-success-border bg-success-container",
      "success",
    );
    await expectPhaseSurface(
      "story:failed",
      "border-af-danger-border bg-error-container",
      "danger",
    );

    for (const paletteId of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteId);
      await expect(document.documentElement.dataset.colorPalette).toBe(
        paletteId,
      );
      const activeNode = (await canvas.findByText("story:review")).closest(
        "article",
      );
      await expect(activeNode).toHaveAttribute(
        "data-graph-visual-emphasis",
        "strong",
      );
    }
    applyDocumentColorPalette("factory-dark");

    const legend = canvasElement.querySelector(
      "[data-factory-graph-work-state-phase-legend]",
    );
    if (!legend) {
      throw new Error("Expected work state phase legend");
    }
    await expect(
      within(legend as HTMLElement).getByText("Initial"),
    ).toBeVisible();
    await expect(
      within(legend as HTMLElement).getByText("Completed"),
    ).toBeVisible();
  },
};

export const ReducedMotion = {
  render: () => <ReducedMotionPreviewStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const activeNode = canvas.getByTestId("reduced-motion-active-node");

    await expect(activeNode).toBeVisible();
    await expect(window.getComputedStyle(activeNode).animationName).toBe(
      "none",
    );
  },
};

export const RuntimeEmphasisStates = {
  render: () => <RuntimeEmphasisStatesStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const idleNode = canvas
      .getByTestId("idle-muted-node")
      .querySelector("article");
    const activeNode = canvas
      .getByTestId("active-selected-node")
      .querySelector("article");
    const failedNode = canvas
      .getByTestId("failed-validation-node")
      .querySelector("article");

    await expect(idleNode).toHaveAttribute("data-graph-visual-status", "quiet");
    await expect(idleNode).toHaveAttribute("data-graph-visual-muted", "true");
    await expect(activeNode).toHaveAttribute(
      "data-graph-visual-status",
      "active",
    );
    await expect(activeNode).toHaveAttribute(
      "data-graph-visual-focus",
      "selection-and-keyboard",
    );
    await expect(activeNode).toHaveClass("ring-af-focus-ring");
    await expect(failedNode).toHaveAttribute(
      "data-graph-visual-status",
      "danger",
    );
    await expect(failedNode).toHaveAttribute(
      "data-graph-visual-validation",
      "error",
    );
    await expect(failedNode).toHaveAttribute("aria-invalid", "true");
  },
};

export const PendingRemoval = {
  render: () => <PendingRemovalStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewNode = await canvas.findByText("review");
    const removingBadge = await canvas.findByText("Removing");
    const edgePath = canvasElement.querySelector(".react-flow__edge-path");

    await expect(reviewNode).toBeVisible();
    await expect(removingBadge).toBeVisible();
    await expect(canvas.getByText("writer")).toBeVisible();
    await expect(canvas.getByText("story:complete")).toBeVisible();
    if (!(edgePath instanceof SVGPathElement)) {
      throw new Error("Expected a React Flow edge path for pending removal.");
    }
    await expect(edgePath.getAttribute("style") ?? "").toContain(
      "var(--color-on-error-container)",
    );
    await expect(edgePath.getAttribute("style") ?? "").toContain(
      "stroke-dasharray: 7, 5",
    );
  },
};

export const ConnectionAnchors = {
  render: () => <ConnectionAnchorsStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const failureSource = await canvas.findByRole("button", {
      name: "Connect: review Failure",
    });
    const failureTarget = await canvas.findByRole("button", {
      name: "Connect: story:queued Failure",
    });
    const continueTarget = await canvas.findByRole("button", {
      name: "Connect: story:queued Continue",
    });

    await expect(failureSource).toBeVisible();
    await expect(failureTarget).toBeVisible();
    await expect(continueTarget).toBeVisible();

    await userEvent.click(failureSource);
    await expect(failureSource).toHaveAttribute("aria-pressed", "true");
  },
};

export const PendingEdgeChanges = {
  render: () => <PendingEdgeChangesStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const failureRoute = await canvas.findByRole("button", {
      name: "Failure route from review to story:queued",
    });
    const successRoute = await canvas.findByRole("button", {
      name: "Success route from review to story:done",
    });
    const edgePaths = Array.from(
      canvasElement.querySelectorAll(".react-flow__edge-path"),
    );

    await expect(canvas.getByText("review")).toBeVisible();
    await expect(failureRoute).toBeVisible();
    await expect(successRoute).toBeVisible();
    await expect(edgePaths).toHaveLength(2);
    await expect(edgePaths[0]?.getAttribute("style") ?? "").toContain(
      "var(--color-on-error-container)",
    );
    await expect(edgePaths[0]?.getAttribute("style") ?? "").toContain(
      "stroke-dasharray: 7, 5",
    );
    await expect(edgePaths[1]?.getAttribute("style") ?? "").toContain(
      "var(--color-on-warning-container)",
    );
    await expect(edgePaths[1]?.getAttribute("style") ?? "").toContain(
      "stroke-dasharray: 9, 4",
    );
  },
};

export const ProgressOutcomeRoutesWithoutStopWords = {
  render: () => (
    <ProgressOutcomeRoutesStory
      workstations={[standardProcessorWithoutStopWords]}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("draft")).toBeVisible();
    await expectProgressOutcomeRouteHandles(canvas, {
      includeContinueAndReject: false,
    });
    await expectZAxisIncompleteHints(canvasElement, { expectHints: false });
  },
};

export const ProgressOutcomeRoutesWithStopWords = {
  render: () => (
    <ProgressOutcomeRoutesStory
      workstations={[standardProcessorWithStopWords]}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("draft")).toBeVisible();
    await expectProgressOutcomeRouteHandles(canvas, {
      includeContinueAndReject: true,
    });
    await expectZAxisIncompleteHints(canvasElement, { expectHints: false });
  },
};

export const ProgressOutcomeRoutesWithWorkerStopToken = {
  render: () => (
    <ProgressOutcomeRoutesStory
      factoryDefinition={factoryWithWorkerStopToken}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("draft")).toBeVisible();
    await expectProgressOutcomeRouteHandles(canvas, {
      includeContinueAndReject: true,
    });
    await expectZAxisIncompleteHints(canvasElement, { expectHints: false });
  },
};

export const LogicalMoveProgressOutcomeHandles = {
  render: () => (
    <ProgressOutcomeRoutesStory
      topology={logicalMoveComparisonTopology}
      workstations={[logicalMoveWorkstation]}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("router")).toBeVisible();
    await expectLogicalMoveConnectHandles(canvas);
    await expectZAxisIncompleteHints(canvasElement, { expectHints: false });
  },
};

export const LogicalMoveComparedToStandardProcessor = {
  render: () => (
    <ProgressOutcomeRoutesStory
      topology={logicalMoveComparisonTopology}
      workstations={[standardProcessorWithoutStopWords, logicalMoveWorkstation]}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("draft")).toBeVisible();
    await expect(canvas.getByText("router")).toBeVisible();
    await expectProgressOutcomeRouteHandles(canvas, {
      includeContinueAndReject: false,
    });
    await expectLogicalMoveConnectHandles(canvas);
    await expectZAxisIncompleteHints(canvasElement, { expectHints: false });
  },
};

export const WorkerResourceDensity = {
  render: () => <WorkerResourceDensityStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const writerNode = await canvas.findByText("writer");
    const reviewerNode = await canvas.findByText("reviewer");
    const stalledNode = await canvas.findByText("stalled");

    await expect(writerNode.closest("article")?.textContent ?? "").toContain(
      "Active",
    );
    await expect(reviewerNode.closest("article")?.textContent ?? "").toContain(
      "Errored",
    );
    await expect(stalledNode.closest("article")?.textContent ?? "").toContain(
      "Unavailable",
    );

    const infrastructurePreset = await canvas.findByRole("button", {
      name: "Infrastructure",
    });
    await userEvent.click(infrastructurePreset);
    await expect(canvas.getByText("gpu")).toBeVisible();
    await expect(canvas.getByText("draft")).toBeVisible();

    const workflowPreset = await canvas.findByRole("button", {
      name: "Workflow",
    });
    await userEvent.click(workflowPreset);
    await expect(canvas.queryByText("writer")).toBeNull();
    await expect(canvas.queryByText("gpu")).toBeNull();
    await expect(canvas.getByText("review")).toBeVisible();
  },
};
