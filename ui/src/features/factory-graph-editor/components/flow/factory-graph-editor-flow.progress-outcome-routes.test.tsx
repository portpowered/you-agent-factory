import "@xyflow/react/dist/style.css";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import "../../../../styles.css";
import { baseFactoryDefinition } from "../../lib/draft/factory-graph-draft.test-helpers";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphTopology,
  FactoryWorkstation,
} from "../../lib/draft/factory-graph-draft-types";
import { projectFactoryValidationTargets } from "../../lib/projection/factory-validation-graph-projection";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "../flow/factory-graph-editor-flow";

function queryHandleByLabel(label: string) {
  return document.querySelector(`[aria-label="${label}"]`);
}

async function findHandleByLabel(label: string) {
  await waitFor(() => {
    expect(queryHandleByLabel(label)).not.toBeNull();
  });

  return queryHandleByLabel(label);
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
  name: "draft",
  stopWords: undefined,
};

const standardProcessorWithStopWords: FactoryWorkstation = {
  ...baseFactoryDefinition.workstations[0],
  behavior: "STANDARD",
  name: "draft",
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
      name: "draft",
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

const routelessLogicalMoveWorkstation: FactoryWorkstation = {
  behavior: "CRON",
  body: "Move work downstream on schedule.",
  cron: {
    schedule: "0 * * * *",
    triggerAtStart: true,
  },
  inputs: [],
  name: "router",
  outputs: [],
  type: "LOGICAL_MOVE",
  worker: "",
};

const routelessLogicalMoveMissingOutputRoutesProjection =
  projectFactoryValidationTargets([
    {
      code: "factory.workstation.missingOutputRoutes",
      message: "missing output routes",
      severity: "error",
      subject: {
        id: "router",
        location: "OUTPUTS",
        type: "WORKSTATION",
      },
    },
  ]);

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

const onRejectionValidationProjection = projectFactoryValidationTargets([
  {
    code: "factory.workstation.missingRejectionRoute",
    message: "missing reject route",
    severity: "error",
    subject: {
      id: "draft",
      location: "ON_REJECTION",
      type: "WORKSTATION",
    },
  },
]);

const routelessCronWorkstation: FactoryWorkstation = {
  behavior: "CRON",
  body: "",
  cron: {
    schedule: "0 * * * *",
    triggerAtStart: true,
  },
  inputs: [],
  name: "cron",
  outputs: [],
  type: "MODEL_WORKSTATION",
  worker: "writer",
};

const routelessCronTopology: FactoryGraphTopology = {
  edges: [],
  nodes: [
    {
      id: "workstation:cron",
      key: { kind: "workstation", name: "cron" },
      kind: "workstation",
      label: "cron",
    },
  ],
};

const missingOutputRoutesValidationProjection = projectFactoryValidationTargets(
  [
    {
      code: "factory.workstation.missingOutputRoutes",
      message: "missing output routes",
      severity: "error",
      subject: {
        id: "cron",
        location: "OUTPUTS",
        type: "WORKSTATION",
      },
    },
  ],
);

function renderProgressOutcomeRouteFlow(
  input:
    | {
        factoryDefinition: CanonicalFactoryDefinition;
        workstations?: never;
      }
    | {
        factoryDefinition?: never;
        workstations: readonly FactoryWorkstation[];
      },
  options?: {
    topology?: FactoryGraphTopology;
    validationProjection?: typeof onRejectionValidationProjection;
  },
) {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: true,
    factoryDefinition:
      "factoryDefinition" in input ? input.factoryDefinition : undefined,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingAdditionNodeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: options?.topology ?? PROGRESS_OUTCOME_ROUTE_TOPOLOGY,
    validationProjection: options?.validationProjection,
    workstations:
      "workstations" in input
        ? input.workstations
        : input.factoryDefinition.workstations,
  });

  return render(
    <div style={{ height: 420, width: 720 }}>
      <ReactFlowProvider>
        <ReactFlow
          edgeTypes={FACTORY_GRAPH_EDITOR_EDGE_TYPES}
          edges={flow.edges}
          fitView
          nodeTypes={FACTORY_GRAPH_EDITOR_NODE_TYPES}
          nodes={flow.nodes}
        >
          <Background />
        </ReactFlow>
      </ReactFlowProvider>
    </div>,
  );
}

function useProgressOutcomeRouteFlowTestHooks() {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });
}

describe("factory graph editor progress outcome route handles", () => {
  useProgressOutcomeRouteFlowTestHooks();

  it("hides continue and reject connect handles for standard processors without stopWords", async () => {
    const { container } = renderProgressOutcomeRouteFlow({
      workstations: [standardProcessorWithoutStopWords],
    });

    await findHandleByLabel("Connect: draft Success");
    expect(queryHandleByLabel("Connect: draft Failure")).not.toBeNull();
    expect(queryHandleByLabel("Connect: draft Continue")).toBeNull();
    expect(queryHandleByLabel("Connect: draft Reject")).toBeNull();

    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("shows continue and reject connect handles when the assigned worker has a stop token", async () => {
    const { container } = renderProgressOutcomeRouteFlow({
      factoryDefinition: factoryWithWorkerStopToken,
    });

    await findHandleByLabel("Connect: draft Continue");
    expect(queryHandleByLabel("Connect: draft Reject")).not.toBeNull();
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("shows continue and reject connect handles when stopWords are configured", async () => {
    const { container } = renderProgressOutcomeRouteFlow({
      workstations: [standardProcessorWithStopWords],
    });

    await findHandleByLabel("Connect: draft Continue");
    expect(queryHandleByLabel("Connect: draft Reject")).not.toBeNull();
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("does not show API validation or z-axis hints on omitted continue and reject handles without stopWords", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      { workstations: [standardProcessorWithoutStopWords] },
      { validationProjection: onRejectionValidationProjection },
    );

    await findHandleByLabel("Connect: draft Success");
    expect(queryHandleByLabel("Connect: draft Continue")).toBeNull();
    expect(queryHandleByLabel("Connect: draft Reject")).toBeNull();
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
    expect(
      container.querySelectorAll(
        '[data-z-axis-incomplete-hint="workstation-on-continue-source"], [data-z-axis-incomplete-hint="workstation-on-rejection-source"]',
      ),
    ).toHaveLength(0);
    expect(
      container.querySelectorAll('[data-node-handle-invalid="true"]'),
    ).toHaveLength(0);
  });

  it("shows missingOutputRoutes validation on the output handle for routeless CRON workstations", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      { workstations: [routelessCronWorkstation] },
      {
        topology: routelessCronTopology,
        validationProjection: missingOutputRoutesValidationProjection,
      },
    );

    const outputHandle = await screen.findByTitle("missing output routes");
    expect(outputHandle.getAttribute("aria-invalid")).toBe("true");
    expect(queryHandleByLabel("missing output routes")).toBe(outputHandle);
    expect(
      container.querySelectorAll('[data-node-handle-invalid="true"]'),
    ).toHaveLength(1);
  });

  it("shows API validation on rendered reject handle when stopWords are configured", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      { workstations: [standardProcessorWithStopWords] },
      { validationProjection: onRejectionValidationProjection },
    );

    const rejectHandle = await screen.findByTitle("missing reject route");
    expect(rejectHandle.getAttribute("aria-invalid")).toBe("true");
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });
});

describe("factory graph editor logical-move progress outcome route handles", () => {
  useProgressOutcomeRouteFlowTestHooks();

  it("hides continue, failure, and reject connect handles on logical-move workstations", async () => {
    renderProgressOutcomeRouteFlow(
      { workstations: [logicalMoveWorkstation] },
      { topology: logicalMoveComparisonTopology },
    );

    await findHandleByLabel("Connect: router Success");
    expect(queryHandleByLabel("Connect: router Input")).not.toBeNull();
    expect(queryHandleByLabel("Connect: router Resource")).not.toBeNull();
    expect(queryHandleByLabel("Connect: router Failure")).toBeNull();
    expect(queryHandleByLabel("Connect: router Continue")).toBeNull();
    expect(queryHandleByLabel("Connect: router Reject")).toBeNull();
  });

  it("shows missingOutputRoutes validation on the output handle for routeless LOGICAL_MOVE workstations", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      { workstations: [routelessLogicalMoveWorkstation] },
      {
        topology: logicalMoveComparisonTopology,
        validationProjection: routelessLogicalMoveMissingOutputRoutesProjection,
      },
    );

    const outputHandle = await screen.findByTitle("missing output routes");
    expect(outputHandle.getAttribute("aria-invalid")).toBe("true");
    expect(queryHandleByLabel("Connect tool: router Failure")).toBeNull();
    expect(queryHandleByLabel("Connect tool: router Continue")).toBeNull();
    expect(queryHandleByLabel("Connect tool: router Reject")).toBeNull();
    expect(
      container.querySelectorAll('[data-node-handle-invalid="true"]'),
    ).toHaveLength(1);
    expect(
      container.querySelectorAll(
        '[data-z-axis-incomplete-hint="workstation-on-failure-source"], [data-z-axis-incomplete-hint="workstation-on-continue-source"], [data-z-axis-incomplete-hint="workstation-on-rejection-source"]',
      ),
    ).toHaveLength(0);
  });

  it("keeps standard processor outcome handles when logical-move is on the same graph", async () => {
    renderProgressOutcomeRouteFlow(
      {
        workstations: [
          standardProcessorWithoutStopWords,
          logicalMoveWorkstation,
        ],
      },
      { topology: logicalMoveComparisonTopology },
    );

    await findHandleByLabel("Connect: draft Success");
    expect(queryHandleByLabel("Connect: draft Failure")).not.toBeNull();
    expect(queryHandleByLabel("Connect: draft Continue")).toBeNull();
    expect(queryHandleByLabel("Connect: draft Reject")).toBeNull();

    await findHandleByLabel("Connect: router Success");
    expect(queryHandleByLabel("Connect: router Failure")).toBeNull();
    expect(queryHandleByLabel("Connect: router Continue")).toBeNull();
    expect(queryHandleByLabel("Connect: router Reject")).toBeNull();
  });
});
