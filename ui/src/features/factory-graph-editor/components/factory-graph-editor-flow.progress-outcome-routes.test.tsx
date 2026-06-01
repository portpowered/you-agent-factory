import "@xyflow/react/dist/style.css";

import { cleanup, render, screen } from "@testing-library/react";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import "../../../styles.css";
import { baseFactoryDefinition } from "../lib/factory-graph-draft.test-helpers";
import type {
  FactoryGraphTopology,
  FactoryWorkstation,
} from "../lib/factory-graph-draft-types";
import { projectFactoryValidationTargets } from "../lib/factory-validation-graph-projection";
import {
  buildFactoryGraphEditorFlowModel,
  FACTORY_GRAPH_EDITOR_EDGE_TYPES,
  FACTORY_GRAPH_EDITOR_NODE_TYPES,
} from "./factory-graph-editor-flow";

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

function renderProgressOutcomeRouteFlow(
  workstations: readonly FactoryWorkstation[],
  options?: {
    topology?: FactoryGraphTopology;
    validationProjection?: typeof onRejectionValidationProjection;
  },
) {
  const flow = buildFactoryGraphEditorFlowModel({
    canEditConnections: true,
    pendingAdditionEdgeIds: new Set<string>(),
    pendingAdditionNodeIds: new Set<string>(),
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: new Set<string>(),
    pendingRemovalNodeIds: new Set<string>(),
    topology: options?.topology ?? PROGRESS_OUTCOME_ROUTE_TOPOLOGY,
    validationProjection: options?.validationProjection,
    workstations,
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

describe("factory graph editor progress outcome route handles", () => {
  let restoreBrowserTestShims: (() => void) | null = null;

  beforeEach(() => {
    restoreBrowserTestShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserTestShims?.();
    restoreBrowserTestShims = null;
  });

  it("hides continue and reject connect handles for standard processors without stopWords", async () => {
    const { container } = renderProgressOutcomeRouteFlow([
      standardProcessorWithoutStopWords,
    ]);

    await screen.findByRole("button", { name: "Connect: draft Success" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Failure" }),
    ).not.toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Continue" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Reject" }),
    ).toBeNull();

    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("shows continue and reject connect handles when stopWords are configured", async () => {
    const { container } = renderProgressOutcomeRouteFlow([
      standardProcessorWithStopWords,
    ]);

    await screen.findByRole("button", { name: "Connect: draft Continue" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Reject" }),
    ).not.toBeNull();
    expect(container.querySelectorAll("[data-z-axis-incomplete-hint]")).toHaveLength(
      0,
    );
  });

  it("does not show API validation or z-axis hints on omitted continue and reject handles without stopWords", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      [standardProcessorWithoutStopWords],
      { validationProjection: onRejectionValidationProjection },
    );

    await screen.findByRole("button", { name: "Connect: draft Success" });
    expect(
      screen.queryByRole("button", { name: "Connect: draft Continue" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Reject" }),
    ).toBeNull();
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
    expect(
      container.querySelectorAll(
        '[data-z-axis-incomplete-hint="workstation-on-continue-source"], [data-z-axis-incomplete-hint="workstation-on-rejection-source"]',
      ),
    ).toHaveLength(0);
    expect(
      container.querySelectorAll(
        '[aria-invalid="true"].ring-af-danger-border',
      ),
    ).toHaveLength(0);
  });

  it("shows API validation on rendered reject handle when stopWords are configured", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      [standardProcessorWithStopWords],
      { validationProjection: onRejectionValidationProjection },
    );

    const rejectHandle = await screen.findByRole("button", {
      name: "missing reject route",
    });
    expect(rejectHandle.className).toContain("ring-af-danger-border");
    expect(rejectHandle.getAttribute("aria-invalid")).toBe("true");
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("hides continue, failure, and reject connect handles on logical-move workstations", async () => {
    renderProgressOutcomeRouteFlow([logicalMoveWorkstation], {
      topology: logicalMoveComparisonTopology,
    });

    await screen.findByRole("button", { name: "Connect: router Success" });
    expect(
      screen.getByRole("button", { name: "Connect: router Input" }),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Connect: router Resource" }),
    ).not.toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: router Failure" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: router Continue" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: router Reject" }),
    ).toBeNull();
  });

  it("keeps standard processor outcome handles when logical-move is on the same graph", async () => {
    renderProgressOutcomeRouteFlow(
      [standardProcessorWithoutStopWords, logicalMoveWorkstation],
      { topology: logicalMoveComparisonTopology },
    );

    await screen.findByRole("button", { name: "Connect: draft Success" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Failure" }),
    ).not.toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Continue" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: draft Reject" }),
    ).toBeNull();

    await screen.findByRole("button", { name: "Connect: router Success" });
    expect(
      screen.queryByRole("button", { name: "Connect: router Failure" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: router Continue" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Connect: router Reject" }),
    ).toBeNull();
  });
});
