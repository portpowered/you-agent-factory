import { useState } from "react";

import { expect, userEvent, within } from "storybook/test";

import "../../styles.css";
import {
  EXHAUSTION_WORKSTATION_ICON_METADATA,
  SUPPORTED_WORKSTATION_ICON_METADATA,
} from "../flowchart";
import {
  resourceOccupancySnapshotForTick,
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
  workstationKindParityExpectations,
  workstationKindParityDashboardSnapshot,
} from "../../components/dashboard/test-fixtures";
import type { FactoryValue } from "../../api/named-factory";
import type { FactoryPngImportValue, ReadFactoryImportFile } from "../import";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";

interface CurrentActivityStoryProps {
  initialSelection?: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}

interface LegendIconExpectation {
  kind: string;
  label: string;
}

const LEGEND_ICON_EXPECTATIONS: LegendIconExpectation[] = [
  { kind: "queue", label: "Queue" },
  { kind: "processing", label: "Processing" },
  { kind: "terminal", label: "Terminal" },
  { kind: "failed", label: "Failed state" },
  { kind: "resource", label: "Resource" },
  { kind: "constraint", label: "Constraint" },
  { kind: "limit", label: "Limit" },
  ...SUPPORTED_WORKSTATION_ICON_METADATA.map((metadata) => ({
    kind: metadata.iconKind,
    label: metadata.label,
  })),
  { kind: "active-work", label: "Active work" },
  {
    kind: EXHAUSTION_WORKSTATION_ICON_METADATA.iconKind,
    label: EXHAUSTION_WORKSTATION_ICON_METADATA.label,
  },
];
const GRAPH_BROWSER_SMOKE_TIMEOUT_MS = 2000;
const GRAPH_BROWSER_SMOKE_POLL_MS = 50;

function snapshotWithStateCounts(overrides: Record<string, number>): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  snapshot.runtime.place_token_counts = {
    ...snapshot.runtime.place_token_counts,
    ...overrides,
  };

  return snapshot;
}

function snapshotWithActiveWorkItemCount(count: number): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
  const reviewActivity = snapshot.runtime.workstation_activity_by_node_id?.review;
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
  const workItems = Array.from({ length: count }, (_, index): DashboardWorkItemRef => {
    const itemNumber = index + 1;

    return {
      display_name: `Active Story ${itemNumber}`,
      trace_id: `trace-active-story-${itemNumber}`,
      work_id: `work-active-story-${itemNumber}`,
      work_type_id: "story",
    };
  });

  if (count === 0) {
    snapshot.runtime.active_dispatch_ids = (snapshot.runtime.active_dispatch_ids ?? []).filter(
      (dispatchID) => dispatchID !== "dispatch-review-active",
    );
    snapshot.runtime.active_workstation_node_ids = (
      snapshot.runtime.active_workstation_node_ids ?? []
    ).filter((nodeID) => nodeID !== "review");
    if (snapshot.runtime.active_executions_by_dispatch_id) {
      delete snapshot.runtime.active_executions_by_dispatch_id["dispatch-review-active"];
    }
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = [];
      reviewActivity.active_work_items = [];
      reviewActivity.trace_ids = [];
    }
  } else if (activeExecution) {
    activeExecution.work_items = workItems;
    activeExecution.trace_ids = workItems.map((workItem) => workItem.trace_id ?? workItem.work_id);
    snapshot.runtime.active_dispatch_ids = ["dispatch-review-active"];
    snapshot.runtime.active_workstation_node_ids = ["review"];
    if (reviewActivity) {
      reviewActivity.active_dispatch_ids = ["dispatch-review-active"];
      reviewActivity.active_work_items = workItems;
      reviewActivity.trace_ids = workItems.map((workItem) => workItem.trace_id ?? workItem.work_id);
    }
  }

  snapshot.runtime.in_flight_dispatch_count = count;

  return snapshot;
}

function snapshotWithLongWorkstationName(): DashboardSnapshot {
  const snapshot = snapshotWithActiveWorkItemCount(0);
  const reviewWorkstation = snapshot.topology.workstation_nodes_by_id.review;

  if (reviewWorkstation) {
    reviewWorkstation.workstation_name = "Review Requests With A Deliberately Long Workstation Title";
  }

  return snapshot;
}

function snapshotWithLongActiveWorkLabel(): DashboardSnapshot {
  const snapshot = snapshotWithActiveWorkItemCount(1);
  const activeExecution =
    snapshot.runtime.active_executions_by_dispatch_id?.["dispatch-review-active"];
  const longWorkLabel =
    "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node";

  if (activeExecution?.work_items?.[0]) {
    activeExecution.work_items[0].display_name = longWorkLabel;
  }

  if (activeExecution?.consumed_tokens?.[0]) {
    activeExecution.consumed_tokens[0].name = longWorkLabel;
  }

  return snapshot;
}

function snapshotWithLongStateLabels(): DashboardSnapshot {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  for (const workstation of Object.values(snapshot.topology.workstation_nodes_by_id)) {
    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      if (place.place_id === "story:ready") {
        place.type_id = "customer-escalation-story-with-a-deliberately-long-type";
        place.state_value = "ready-for-review-after-multiple-dependent-checks-complete";
      }
    }
  }

  return snapshot;
}

function createFactoryImportValue(): FactoryPngImportValue {
  return {
    factory: {
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    },
    previewImageSrc: "blob:factory-preview",
    revokePreviewImageSrc: () => {},
    schemaVersion: "portos.agent-factory.png.v1",
  };
}

function stateArticle(button: HTMLElement): HTMLElement {
  const article = button.closest("article");

  if (!(article instanceof HTMLElement)) {
    throw new Error("Expected state-position button to render inside an article");
  }

  return article;
}

function workstationNode(button: HTMLElement): HTMLElement {
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error("Expected workstation button to render inside a React Flow node");
  }

  return node;
}

function expectFixedWorkstationDimensions(node: HTMLElement): void {
  expect(node.getAttribute("style")).toContain("width: 156px");
  expect(node.getAttribute("style")).toContain("height: 196px");
}

function expectNoImplementationLabels(canvasElement: HTMLElement): void {
  const canvas = within(canvasElement);

  expect(canvas.queryByText("Workstation Definition")).not.toBeInTheDocument();
  expect(canvas.queryByText("State Position")).not.toBeInTheDocument();
}

async function expectResourceCount(
  canvasElement: HTMLElement,
  count: number,
): Promise<void> {
  const canvas = within(canvasElement);
  const resourceCount = await canvas.findByLabelText(`${count} resource tokens`);

  await expect(await canvas.findByLabelText("agent-slot:available")).toBeVisible();
  await expect(resourceCount).toBeVisible();
  await expect(resourceCount).toHaveTextContent(String(count));
  expectNoImplementationLabels(canvasElement);
}

async function findGraphElement(
  canvasElement: HTMLElement,
  selector: string,
  errorMessage: string,
): Promise<Element> {
  const deadline = Date.now() + GRAPH_BROWSER_SMOKE_TIMEOUT_MS;

  while (Date.now() <= deadline) {
    const element = canvasElement.querySelector(selector);
    if (element instanceof Element) {
      return element;
    }

    await new Promise((resolve) => {
      window.setTimeout(resolve, GRAPH_BROWSER_SMOKE_POLL_MS);
    });
  }

  throw new Error(errorMessage);
}

async function expectLegendIconVocabulary(canvasElement: HTMLElement): Promise<void> {
  const legend = await expandGraphLegend(canvasElement);
  const legendScope = within(legend);

  await expect(legend).toBeVisible();
  for (const item of LEGEND_ICON_EXPECTATIONS) {
    const icon = legendScope.getByRole("img", { name: `${item.label} legend icon` });

    await expect(icon).toBeVisible();
    await expect(icon).toHaveAttribute("data-graph-semantic-icon", item.kind);
    await expect(legendScope.getByText(item.label)).toBeVisible();
  }
}

async function expandGraphLegend(canvasElement: HTMLElement): Promise<HTMLElement> {
  const canvas = within(canvasElement);
  const existingLegend = canvas.queryByLabelText("Graph legend");

  if (existingLegend instanceof HTMLElement) {
    return existingLegend;
  }

  const expandButton = await canvas.findByRole("button", { name: "Expand graph legend" });

  await expect(expandButton).toHaveAttribute("aria-expanded", "false");
  await userEvent.click(expandButton);

  const legend = await canvas.findByLabelText("Graph legend");

  await expect(canvas.getByRole("button", { name: "Collapse graph legend" })).toHaveAttribute(
    "aria-expanded",
    "true",
  );

  return legend;
}

async function expectGraphBrowserSmoke(canvasElement: HTMLElement): Promise<void> {
  const canvas = within(canvasElement);
  const controls = await findGraphElement(
    canvasElement,
    ".react-flow__controls",
    "Expected React Flow controls to render",
  );
  const edge = await findGraphElement(
    canvasElement,
    ".react-flow__edge-path",
    "Expected React Flow edges to render",
  );

  if (!(controls instanceof HTMLElement)) {
    throw new Error("Expected React Flow controls to render");
  }

  await expect(canvas.getByRole("region", { name: "Work graph viewport" })).toBeVisible();
  await expect(controls).toBeVisible();
  await expect(edge).toBeVisible();
}

function expectNoPageHorizontalOverflow(canvasElement: HTMLElement): void {
  const documentElement = canvasElement.ownerDocument.documentElement;
  const overflowTolerance = 1;

  expect(documentElement.scrollWidth <= documentElement.clientWidth + overflowTolerance).toBe(true);
}

function CurrentActivityStory({
  initialSelection = null,
  snapshot,
}: CurrentActivityStoryProps) {
  const [selection, setSelection] = useState<CurrentActivitySelection | null>(initialSelection);

  return (
    <div style={{ minHeight: "760px" }}>
      <ReactFlowCurrentActivityCard
        now={Date.parse("2026-04-08T12:00:04Z")}
        selection={selection}
        snapshot={snapshot}
        onSelectWorkItem={(
          dispatchId: string,
          nodeId: string,
          _execution: DashboardActiveExecution,
          workItem: DashboardWorkItemRef,
        ) =>
          setSelection({
            kind: "work-item",
            dispatchId,
            nodeId,
            workID: workItem.work_id,
          })
        }
        onSelectWorkstation={(nodeId) => setSelection({ kind: "node", nodeId })}
        onSelectStateNode={(placeId) => setSelection({ kind: "state-node", placeId })}
      />
    </div>
  );
}

function CurrentActivityImportStory({ snapshot }: CurrentActivityStoryProps) {
  const [selection, setSelection] = useState<CurrentActivitySelection | null>(null);
  const [activationStatus, setActivationStatus] = useState("No factory activated yet.");
  const [importValue] = useState(() => createFactoryImportValue());

  return (
    <>
      <div style={{ minHeight: "760px" }}>
        <ReactFlowCurrentActivityCard
          activateFactory={async (value: FactoryValue) => value}
          now={Date.parse("2026-04-08T12:00:04Z")}
          onFactoryActivated={() => {
            setActivationStatus(`Activated factory: ${importValue.factory.name}`);
          }}
          onSelectWorkItem={(
            dispatchId: string,
            nodeId: string,
            _execution: DashboardActiveExecution,
            workItem: DashboardWorkItemRef,
          ) =>
            setSelection({
              kind: "work-item",
              dispatchId,
              nodeId,
              workID: workItem.work_id,
            })
          }
          onSelectWorkstation={(nodeId) => setSelection({ kind: "node", nodeId })}
          onSelectStateNode={(placeId) => setSelection({ kind: "state-node", placeId })}
          readFactoryImportFile={async (_file: File) => ({
            ok: true,
            value: importValue,
          }) satisfies Awaited<ReturnType<ReadFactoryImportFile>>}
          selection={selection}
          snapshot={snapshot}
        />
      </div>
      <p>{activationStatus}</p>
    </>
  );
}

export default {
  title: "Agent Factory/Dashboard/React Flow Current Activity Card",
  component: ReactFlowCurrentActivityCard,
};

export const SemanticWorkflow = {
  render: () => <CurrentActivityStory snapshot={semanticWorkflowDashboardSnapshot} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("button", { name: "Select Review workstation" }),
    ).toBeVisible();
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });
    await expect(
      within(reviewButton).getByRole("img", { name: "Repeater workstation" }),
    ).toBeVisible();
    await expect(
      within(await canvas.findByRole("button", { name: "Select Plan workstation" })).getByRole(
        "img",
        { name: "Standard workstation" },
      ),
    ).toBeVisible();
    await expect(within(reviewButton).getByRole("img", { name: "Active" })).toBeVisible();
    await expect(await canvas.findByText("Active Story")).toBeVisible();
    await expect(canvas.queryByText("Workstation Definition")).not.toBeInTheDocument();
    await expect(await canvas.findByText("quality-gate:ready")).toBeVisible();
    await expect(await canvas.findByLabelText("1 constraint token")).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Select story:blocked state" }),
    ).toBeVisible();
    const activeStateButton = await canvas.findByRole("button", {
      name: "Select story:complete state",
    });
    const edgeStyles = Array.from(
      canvasElement.querySelectorAll<SVGPathElement>(".react-flow__edge-path"),
    ).map((edgePath) => edgePath.getAttribute("style") ?? "");

    await expect(edgeStyles.length).toBeGreaterThan(0);
    await expect(edgeStyles.some((style) => style.includes("var(--color-af-edge-muted"))).toBe(
      true,
    );
    await expect(edgeStyles.some((style) => style.includes("var(--color-af-success)"))).toBe(
      true,
    );
    await expect(activeStateButton.closest("article")?.className).toContain(
      "border-af-success/70",
    );
    await expect(canvas.queryByText("Active flow")).not.toBeInTheDocument();
    await expectLegendIconVocabulary(canvasElement);
    await expectGraphBrowserSmoke(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
    await userEvent.click((await canvas.findAllByRole("button", { name: /Active Story/ }))[0]);
    await expect((await canvas.findAllByRole("button", { name: /Active Story/ }))[0]).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  },
};

export const FactoryImportPreviewActivation = {
  render: () => <CurrentActivityImportStory snapshot={semanticWorkflowDashboardSnapshot} />,
};

export const SingleNode = {
  render: () => <CurrentActivityStory snapshot={singleNodeDashboardSnapshot} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("button", { name: "Select Intake workstation" }),
    ).toBeVisible();
    await expect(canvas.queryByText("Idle")).not.toBeInTheDocument();
  },
};

export const WorkstationIdle = {
  render: () => <CurrentActivityStory snapshot={snapshotWithActiveWorkItemCount(0)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = workstationNode(reviewButton);

    expectFixedWorkstationDimensions(reviewNode);
    await expect(
      within(reviewButton).getByRole("img", { name: "Repeater workstation" }),
    ).toBeVisible();
    await expect(reviewNode.querySelector("[data-active='true']")).not.toBeInTheDocument();
    await expect(within(reviewNode).queryByRole("img", { name: "Active" })).not.toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: /Active Story/ })).not.toBeInTheDocument();
    expectNoImplementationLabels(canvasElement);
  },
};

export const WorkstationOneActive = {
  render: () => <CurrentActivityStory snapshot={snapshotWithActiveWorkItemCount(1)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = workstationNode(reviewButton);

    expectFixedWorkstationDimensions(reviewNode);
    await expect(within(reviewButton).getByRole("img", { name: "Active" })).toBeVisible();
    await expect(await canvas.findByRole("button", { name: /Active Story 1/ })).toBeVisible();
    await expect(reviewNode.querySelector("[data-workstation-work-progress]")).not.toBeInTheDocument();
    expectNoImplementationLabels(canvasElement);
  },
};

export const WorkstationFiveActive = {
  render: () => <CurrentActivityStory snapshot={snapshotWithActiveWorkItemCount(5)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = workstationNode(reviewButton);

    expectFixedWorkstationDimensions(reviewNode);
    await expect(await canvas.findByRole("button", { name: /Active Story 1/ })).toBeVisible();
    await expect(await canvas.findByRole("button", { name: /Active Story 3/ })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: /Active Story 4/ })).not.toBeInTheDocument();
    await expect(within(reviewNode).getByLabelText("5 active items")).toBeVisible();
    await expect(within(reviewNode).getByText("+2")).toBeVisible();
    expectNoImplementationLabels(canvasElement);
  },
};

export const HighOccupancyWorkstation = {
  render: () => <CurrentActivityStory snapshot={snapshotWithActiveWorkItemCount(6)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });
    const reviewNode = workstationNode(reviewButton);

    expectFixedWorkstationDimensions(reviewNode);
    await expect(await canvas.findByLabelText("6 active items")).toBeVisible();
    await expect(await canvas.findByRole("button", { name: /Active Story 1/ })).toBeVisible();
    await expect(canvas.queryByRole("button", { name: /Active Story 4/ })).not.toBeInTheDocument();
    await userEvent.click(reviewButton);
    await expect(reviewButton).toHaveAttribute("aria-pressed", "true");
  },
};

export const WorkstationSelected = {
  render: () => (
    <CurrentActivityStory
      initialSelection={{ kind: "node", nodeId: "review" }}
      snapshot={snapshotWithActiveWorkItemCount(1)}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });
    const article = reviewButton.closest("article");

    await expect(reviewButton).toHaveAttribute("aria-pressed", "true");
    await expect(article?.className).toContain("border-af-accent/70");
    expectFixedWorkstationDimensions(workstationNode(reviewButton));
  },
};

export const WorkstationSelectedWorkItem = {
  render: () => (
    <CurrentActivityStory
      initialSelection={{
        dispatchId: "dispatch-review-active",
        kind: "work-item",
        nodeId: "review",
        workID: "work-active-story-1",
      }}
      snapshot={snapshotWithActiveWorkItemCount(1)}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const selectedWorkButton = await canvas.findByRole("button", { name: /Active Story 1/ });
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });

    await expect(selectedWorkButton).toHaveAttribute("aria-pressed", "true");
    await expect(selectedWorkButton).toHaveAttribute("data-selected", "true");
    expectFixedWorkstationDimensions(workstationNode(reviewButton));
  },
};

export const WorkstationLongName = {
  render: () => <CurrentActivityStory snapshot={snapshotWithLongWorkstationName()} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const longNameButton = await canvas.findByRole("button", {
      name: /Select Review Requests With A Deliberately Long Workstation Title workstation/,
    });
    const label = longNameButton.querySelector("[data-workstation-title]");

    expectFixedWorkstationDimensions(workstationNode(longNameButton));
    await expect(longNameButton).toHaveAttribute(
      "title",
      "Review Requests With A Deliberately Long Workstation Title",
    );
    expect(label?.className).toContain("truncate");
    expect(label?.className).toContain("whitespace-nowrap");
    expectNoImplementationLabels(canvasElement);
  },
};

export const WorkstationKindParity = {
  render: () => <CurrentActivityStory snapshot={workstationKindParityDashboardSnapshot} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const legend = await expandGraphLegend(canvasElement);
    const legendScope = within(legend);

    for (const expectation of workstationKindParityExpectations) {
      const button = await canvas.findByRole("button", {
        name: expectation.buttonName,
      });
      const icon = within(button).getByRole("img", { name: expectation.metadata.label });
      const legendIcon = legendScope.getByRole("img", {
        name: `${expectation.metadata.label} legend icon`,
      });

      expectFixedWorkstationDimensions(workstationNode(button));
      await expect(icon).toBeVisible();
      await expect(icon).toHaveAttribute("data-graph-semantic-icon", expectation.metadata.iconKind);
      await expect(within(button).getByText(expectation.workstationName)).toBeVisible();
      await expect(legendIcon).toBeVisible();
      await expect(legendIcon).toHaveAttribute(
        "data-graph-semantic-icon",
        expectation.metadata.iconKind,
      );
      await expect(legendScope.getByText(expectation.metadata.label)).toBeVisible();
      expect(button.textContent).not.toContain(expectation.metadata.label);
    }

    const cronExpectation = workstationKindParityExpectations.find(
      (expectation) => expectation.nodeID === "nightly-cron",
    );
    const cronButton = await canvas.findByRole("button", {
      name: cronExpectation?.buttonName ?? "Select Nightly Cron workstation",
    });

    await expect(cronButton).toHaveAttribute("title", "Nightly Cron");
    await userEvent.click(cronButton);
    await expect(cronButton).toHaveAttribute("aria-pressed", "true");
    expectNoImplementationLabels(canvasElement);
  },
};

export const WorkstationLongWorkItemLabel = {
  render: () => <CurrentActivityStory snapshot={snapshotWithLongActiveWorkLabel()} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const longWorkButton = await canvas.findByRole("button", {
      name: /Active Story With A Deliberately Long Label/,
    });
    const [workLabel, durationLabel] = Array.from(longWorkButton.querySelectorAll("span"));
    const reviewButton = await canvas.findByRole("button", {
      name: "Select Review workstation",
    });

    expectFixedWorkstationDimensions(workstationNode(reviewButton));
    expect(longWorkButton.className).toContain("overflow-hidden");
    expect(workLabel?.className).toContain("truncate");
    expect(durationLabel?.textContent).toBe("4s");
    await userEvent.click(longWorkButton);
    await expect(longWorkButton).toHaveAttribute("aria-pressed", "true");
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionIdle = {
  render: () => (
    <CurrentActivityStory snapshot={snapshotWithStateCounts({ "story:documented": 0 })} />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const stateButton = await canvas.findByRole("button", {
      name: "Select story:documented state",
    });
    const article = stateArticle(stateButton);

    await expect(within(article).getByRole("img", { name: "Processing state" })).toBeVisible();
    await expect(within(article).getByText("story")).toBeVisible();
    await expect(within(article).getByText("documented")).toBeVisible();
    expect(article.querySelector("[data-state-label-zone]")).not.toBeNull();
    expect(article.querySelector("[data-state-marker-zone]")).not.toBeNull();
    expect(article.querySelector("[data-state-work-progress-dot]")).toBeNull();
    expect(within(article).getByText("0 active items")).toBeInTheDocument();
    expect(article.textContent).not.toContain("Queue");
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionOneActive = {
  render: () => <CurrentActivityStory snapshot={snapshotWithStateCounts({ "story:ready": 1 })} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const stateButton = await canvas.findByRole("button", {
      name: "Select story:ready state",
    });
    const article = stateArticle(stateButton);

    await expect(within(article).getByText("story")).toBeVisible();
    await expect(within(article).getByText("ready")).toBeVisible();
    await expect(within(article).getByLabelText("1 active item")).toBeVisible();
    expect(article.querySelectorAll("[data-state-work-progress-dot]")).toHaveLength(1);
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionTenActive = {
  render: () => <CurrentActivityStory snapshot={snapshotWithStateCounts({ "story:ready": 10 })} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const stateButton = await canvas.findByRole("button", {
      name: "Select story:ready state",
    });
    const article = stateArticle(stateButton);

    await expect(within(article).getByLabelText("10 active items")).toBeVisible();
    expect(article.querySelectorAll("[data-state-work-progress-dot]")).toHaveLength(10);
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionNumericOverflow = {
  render: () => <CurrentActivityStory snapshot={snapshotWithStateCounts({ "story:ready": 11 })} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const stateButton = await canvas.findByRole("button", {
      name: "Select story:ready state",
    });
    const article = stateArticle(stateButton);

    await expect(within(article).getByLabelText("11 active items")).toBeVisible();
    await expect(within(article).getByText("11")).toBeVisible();
    expect(article.querySelector("[data-state-work-progress='dots']")).toBeNull();
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionSelected = {
  render: () => (
    <CurrentActivityStory
      initialSelection={{ kind: "state-node", placeId: "story:ready" }}
      snapshot={snapshotWithStateCounts({ "story:ready": 3 })}
    />
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const stateButton = await canvas.findByRole("button", {
      name: "Select story:ready state",
    });
    const article = stateArticle(stateButton);

    await expect(stateButton).toHaveAttribute("aria-pressed", "true");
    await expect(stateButton).toHaveAttribute("data-selected-state", "true");
    await expect(within(article).getByLabelText("3 active items")).toBeVisible();
    await expect(article.className).toContain("border-af-accent/70");
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionLongLabels = {
  render: () => <CurrentActivityStory snapshot={snapshotWithLongStateLabels()} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const stateButton = await canvas.findByRole("button", {
      name: "Select customer-escalation-story-with-a-deliberately-long-type:ready-for-review-after-multiple-dependent-checks-complete state",
    });
    const article = stateArticle(stateButton);
    const workType = article.querySelector("[data-state-work-type]");
    const stateValue = article.querySelector("[data-state-value]");

    expect(article.querySelector("[data-state-label-zone]")?.className).toContain(
      "overflow-hidden",
    );
    expect(workType?.getAttribute("title")).toBe(
      "customer-escalation-story-with-a-deliberately-long-type",
    );
    expect(stateValue?.getAttribute("title")).toBe(
      "ready-for-review-after-multiple-dependent-checks-complete",
    );
    expect(article.querySelector("[data-state-marker-zone]")).not.toBeNull();
    expectNoImplementationLabels(canvasElement);
  },
};

export const StatePositionTerminalAndFailed = {
  render: () => <CurrentActivityStory snapshot={semanticWorkflowDashboardSnapshot} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const terminalButton = await canvas.findByRole("button", {
      name: "Select story:complete state",
    });
    const failedButton = await canvas.findByRole("button", {
      name: "Select story:blocked state",
    });
    const terminalArticle = stateArticle(terminalButton);
    const failedArticle = stateArticle(failedButton);

    await expect(within(terminalArticle).getByRole("img", { name: "Terminal" })).toBeVisible();
    await expect(within(failedArticle).getByRole("img", { name: "Failed" })).toBeVisible();
    await expect(within(terminalArticle).getByText("complete")).toBeVisible();
    await expect(within(failedArticle).getByText("blocked")).toBeVisible();
    expect(terminalArticle.textContent).not.toContain("Terminal");
    expect(failedArticle.textContent).not.toContain("Failed");
    const legend = await expandGraphLegend(canvasElement);
    await expect(
      within(legend).getByRole("img", {
        name: "Terminal legend icon",
      }),
    ).toBeVisible();
    await expect(
      within(legend).getByRole("img", {
        name: "Failed state legend icon",
      }),
    ).toBeVisible();
    expectNoImplementationLabels(canvasElement);
  },
};

export const ResourceIdleCapacity = {
  render: () => <CurrentActivityStory snapshot={resourceOccupancySnapshotForTick(1)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectResourceCount(canvasElement, 2);
  },
};

export const ResourceActiveDispatch = {
  render: () => <CurrentActivityStory snapshot={resourceOccupancySnapshotForTick(3)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expectResourceCount(canvasElement, 1);
    await expect(
      await canvas.findByRole("button", { name: /Resource Occupancy Story/ }),
    ).toBeVisible();
  },
};

export const ResourceReleasedCapacity = {
  render: () => <CurrentActivityStory snapshot={resourceOccupancySnapshotForTick(4)} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectResourceCount(canvasElement, 2);
  },
};

export const LongActiveWorkLabel = {
  render: () => <CurrentActivityStory snapshot={snapshotWithLongActiveWorkLabel()} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const longWorkItem = await canvas.findByRole("button", {
      name: /Active Story With A Deliberately Long Label/,
    });
    const label = longWorkItem.querySelector("[data-active-work-label]");
    const duration = longWorkItem.querySelector("[data-active-work-duration]");

    await expect(label).toBeVisible();
    await expect(label).toHaveTextContent(
      "Active Story With A Deliberately Long Label That Must Stay Inside The Workstation Node",
    );
    await expect(duration).toBeVisible();
    await expect(duration).toHaveTextContent("4s");
    await expect(longWorkItem).toHaveAttribute("aria-pressed", "false");

    await userEvent.click(longWorkItem);

    await expect(longWorkItem).toHaveAttribute("aria-pressed", "true");
  },
};

export const TwentyNode = {
  render: () => <CurrentActivityStory snapshot={twentyNodeDashboardSnapshot} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      await canvas.findByRole("button", { name: "Select Station 20 workstation" }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Select story:step-6 state" }),
    ).toBeVisible();
    await expect((await canvas.findAllByRole("img", { name: "Queue" }))[0]).toBeVisible();
    await expandGraphLegend(canvasElement);
    await expect(
      await canvas.findByRole("img", { name: "Standard workstation legend icon" }),
    ).toBeVisible();
    await expectLegendIconVocabulary(canvasElement);
    await expectGraphBrowserSmoke(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const NarrowViewport = {
  render: () => (
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <CurrentActivityStory snapshot={semanticWorkflowDashboardSnapshot} />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const frame = canvasElement.firstElementChild;

    await expect(
      await canvas.findByRole("button", { name: "Select Review workstation" }),
    ).toBeVisible();
    await expectLegendIconVocabulary(canvasElement);
    await expectGraphBrowserSmoke(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
    expect(frame?.getBoundingClientRect().width ?? 0).toBeLessThanOrEqual(360);
  },
};
