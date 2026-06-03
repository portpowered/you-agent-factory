import "@xyflow/react/dist/style.css";

import { cleanup, render, screen } from "@testing-library/react";
import { Background, ReactFlow, ReactFlowProvider } from "@xyflow/react";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import "../../../styles.css";
import { baseFactoryDefinition } from "../lib/factory-graph-draft.test-helpers";
import type {
  CanonicalFactoryDefinition,
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

  it("shows continue and reject connect handles when the assigned worker has a stop token", async () => {
    const { container } = renderProgressOutcomeRouteFlow({
      factoryDefinition: factoryWithWorkerStopToken,
    });

    await screen.findByRole("button", { name: "Connect: draft Continue" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Reject" }),
    ).not.toBeNull();
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("shows continue and reject connect handles when stopWords are configured", async () => {
    const { container } = renderProgressOutcomeRouteFlow({
      workstations: [standardProcessorWithStopWords],
    });

    await screen.findByRole("button", { name: "Connect: draft Continue" });
    expect(
      screen.getByRole("button", { name: "Connect: draft Reject" }),
    ).not.toBeNull();
    expect(
      container.querySelectorAll("[data-z-axis-incomplete-hint]"),
    ).toHaveLength(0);
  });

  it("does not show API validation or z-axis hints on omitted continue and reject handles without stopWords", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      { workstations: [standardProcessorWithoutStopWords] },
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
      container.querySelectorAll('[aria-invalid="true"].ring-af-danger-border'),
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

    const outputHandle = await screen.findByRole("button", {
      name: "missing output routes",
    });
    expect(outputHandle.className).toContain("ring-af-danger-border");
    expect(outputHandle.getAttribute("aria-invalid")).toBe("true");
    expect(
      screen.queryByRole("button", {
        name: /missing output routes|missing failure route/i,
      }),
    ).toBe(outputHandle);
    expect(
      container.querySelectorAll('[aria-invalid="true"].ring-af-danger-border'),
    ).toHaveLength(1);
  });

  it("shows API validation on rendered reject handle when stopWords are configured", async () => {
    const { container } = renderProgressOutcomeRouteFlow(
      { workstations: [standardProcessorWithStopWords] },
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
});

describe("factory graph editor logical-move progress outcome route handles", () => {
  useProgressOutcomeRouteFlowTestHooks();

  it("hides continue, failure, and reject connect handles on logical-move workstations", async () => {
    renderProgressOutcomeRouteFlow(
      { workstations: [logicalMoveWorkstation] },
      { topology: logicalMoveComparisonTopology },
    );

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
      {
        workstations: [
          standardProcessorWithoutStopWords,
          logicalMoveWorkstation,
        ],
      },
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
