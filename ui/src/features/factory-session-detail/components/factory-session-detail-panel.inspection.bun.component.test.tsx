import { describe, expect, it } from "bun:test";
import { fireEvent, render, screen } from "@testing-library/react";
import type { FactoryEvent } from "../../../api/events";
import { parseFactoryEventReplayStream } from "../../../api/factory-sessions/event-replay";
import {
  FactoryOrchestratorKind,
  FactorySessionDurableLifecycleStatus,
  FactorySessionJavaScriptScriptStatus,
  FactorySessionStatus,
} from "../../../api/generated/openapi";
import { buildSuccessfulReplayEventStream } from "../../../testing/factory-session-event-replay-fixtures";
import type { FactorySessionArtifactDrilldownViewState } from "../hooks/use-factory-session-artifact-drilldown";
import type {
  FactorySessionDetailData,
  FactorySessionDetailViewState,
} from "../hooks/use-factory-session-detail";
import type { FactorySessionDispatchDetailViewState } from "../hooks/use-factory-session-dispatch-detail";
import type { FactorySessionLifecycleControl } from "../hooks/use-factory-session-lifecycle-control";
import type { FactorySessionArtifactDrilldown } from "../lib/factory-session-artifact-drilldown";
import type { FactorySessionDispatchDrilldownModel } from "../lib/factory-session-dispatch-detail";
import {
  type FactorySessionDetailInspectionState,
  FactorySessionDetailPanel,
} from "./factory-session-detail-panel";

const SESSION_ID = "dur-sess-inspection-001";
const PROVIDER_DISPATCH_ID = "dispatch-provider-001";
const FAILED_DISPATCH_ID = "dispatch-failed-001";
const ARTIFACT_ID = "artifact-provider-001";

const replayEvents: FactoryEvent[] = parseFactoryEventReplayStream(
  buildSuccessfulReplayEventStream(SESSION_ID),
);

const detailState: FactorySessionDetailViewState = {
  data: {
    dispatches: [
      {
        artifactIds: [ARTIFACT_ID],
        dispatchKind: "JAVASCRIPT_AGENT",
        id: PROVIDER_DISPATCH_ID,
        javascript: {
          executionMode: "live",
          taskKind: "AGENT",
          taskLabel: "Provider task",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        providerSessionRefs: [
          {
            id: "provider-session-001",
            kind: "session_id",
            provider: "codex",
          },
        ],
        sessionId: SESSION_ID,
        status: "COMPLETED",
      },
      {
        dispatchKind: "JAVASCRIPT_VERIFY",
        failureDetail: {
          message: "Provider session timed out.",
          reason: "PROVIDER_TIMEOUT",
        },
        id: FAILED_DISPATCH_ID,
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        sessionId: SESSION_ID,
        status: "FAILED",
      },
    ],
    durableLifecycleStatus:
      FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded,
    session: {
      factoryDir: "/workspace/root/inspection",
      folderPath: "/workspace/root",
      id: SESSION_ID,
      isDefault: false,
      project: "inspection",
      runtime: {
        artifacts: [
          {
            id: ARTIFACT_ID,
            kind: "CHILD_RESULT",
            label: "Provider transcript",
            visibility: "CUSTOMER",
          },
        ],
        javascript: {
          childDispatchCounts: {
            completed: 1,
            queued: 0,
            running: 0,
          },
          phase: "review",
          phases: ["review"],
          scriptStatus: FactorySessionJavaScriptScriptStatus.IDLE,
        },
        lifecycle: {
          startedAt: "2026-08-11T18:00:00Z",
          updatedAt: "2026-08-11T18:05:00Z",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        progress: {
          categories: {
            failed: 1,
            initial: 0,
            processing: 0,
            terminal: 1,
          },
          factoryState: "RUNNING",
          inFlightCount: 0,
          totalTokens: 0,
        },
        status: FactorySessionStatus.FINISHED,
        usage: { resources: [] },
      },
      target: { kind: "named", name: "inspection" },
    },
  } as FactorySessionDetailData,
  status: "success",
};

const providerDispatchDetail: FactorySessionDispatchDrilldownModel = {
  artifactLinks: [
    {
      href: `/factory-sessions/${SESSION_ID}/artifacts/${ARTIFACT_ID}`,
      id: ARTIFACT_ID,
    },
  ],
  dispatchID: PROVIDER_DISPATCH_ID,
  dispatchKind: "JAVASCRIPT_AGENT",
  javascript: {
    executionMode: "live",
    taskKind: "AGENT",
    taskLabel: "Provider task",
  },
  label: "Provider task",
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  provider: "codex",
  providerSessionRefs: [
    {
      id: "provider-session-001",
      kind: "session_id",
      provider: "codex",
    },
  ],
  relatedWorkIDs: ["work-provider-001"],
  sessionID: SESSION_ID,
  status: "COMPLETED",
  statusHistory: ["QUEUED", "RUNNING", "COMPLETED"],
  warnings: [],
};

const failedDispatchDetail: FactorySessionDispatchDrilldownModel = {
  artifactLinks: [],
  dispatchID: FAILED_DISPATCH_ID,
  dispatchKind: "JAVASCRIPT_VERIFY",
  failureDetail: {
    message: "Provider session timed out.",
    reason: "PROVIDER_TIMEOUT",
  },
  javascript: {
    executionMode: "live",
    taskKind: "VERIFY",
    taskLabel: "Provider verification",
  },
  orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
  providerSessionRefs: [],
  relatedWorkIDs: [],
  sessionID: SESSION_ID,
  status: "FAILED",
  statusHistory: ["QUEUED", "RUNNING", "FAILED"],
  warnings: [],
};

const artifactDrilldown: FactorySessionArtifactDrilldown = {
  artifactId: ARTIFACT_ID,
  capture: {
    capturedAt: "2026-08-11T18:04:00Z",
    mimeType: "text/plain",
    sourceDispatchId: PROVIDER_DISPATCH_ID,
  },
  createdAt: "2026-08-11T18:04:00Z",
  dispatchId: PROVIDER_DISPATCH_ID,
  kind: "CHILD_RESULT",
  label: "Provider transcript",
  preview: { kind: "unavailable" },
  sessionId: SESSION_ID,
  sizeBytes: 128,
  summary: "Captured provider output.",
  visibility: "CUSTOMER",
};

function createInspectionState(
  dispatchDetails: Record<string, FactorySessionDispatchDetailViewState>,
  overrides: Partial<FactorySessionDetailInspectionState> = {},
): FactorySessionDetailInspectionState {
  return {
    artifactDrilldowns: {
      [ARTIFACT_ID]: {
        artifact: artifactDrilldown,
        status: "success",
      } satisfies FactorySessionArtifactDrilldownViewState,
    },
    dispatchDetails,
    eventReplay: { events: replayEvents, status: "success" },
    ...overrides,
  };
}

function createLifecycleControl(): FactorySessionLifecycleControl {
  return {
    feedback: null,
    pendingActionID: null,
    submitLifecycleAction: async () => undefined,
  };
}

function renderPanel(inspectionState: FactorySessionDetailInspectionState) {
  return render(
    <FactorySessionDetailPanel
      detailState={detailState}
      inspectionState={inspectionState}
      lifecycleControl={createLifecycleControl()}
      sessionID={SESSION_ID}
    />,
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: inspection states share one focused panel contract.
describe("FactorySessionDetailPanel inspection inputs", () => {
  it("keeps provider, artifact, failure, and replay drill-downs at their owning boundaries", () => {
    renderPanel(
      createInspectionState({
        [PROVIDER_DISPATCH_ID]: {
          data: providerDispatchDetail,
          status: "success",
        },
        [FAILED_DISPATCH_ID]: {
          data: failedDispatchDetail,
          status: "success",
        },
      }),
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${PROVIDER_DISPATCH_ID}`,
      }),
    );
    expect(screen.getByText("Provider sessions")).toBeTruthy();
    expect(screen.getByText("session_id · provider-session-001")).toBeTruthy();
    expect(screen.getByRole("link", { name: ARTIFACT_ID })).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: `View artifact ${ARTIFACT_ID}` }),
    );
    expect(
      screen.getByRole("heading", { name: "Artifact detail" }),
    ).toBeTruthy();
    expect(screen.getByText("Captured provider output.")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );
    expect(screen.getByText("Session completed")).toBeTruthy();
    expect(screen.getByText("Showing 5 Factory Events.")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", {
        name: `Collapse dispatch detail for ${PROVIDER_DISPATCH_ID}`,
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${FAILED_DISPATCH_ID}`,
      }),
    );
    expect(screen.getByText("Provider session timed out.")).toBeTruthy();
    expect(screen.getByText("PROVIDER_TIMEOUT")).toBeTruthy();
  });

  it("renders explicit dispatch loading, unavailable, and retryable error states", () => {
    const retryIDs: string[] = [];
    const { rerender } = renderPanel(
      createInspectionState({
        [PROVIDER_DISPATCH_ID]: { status: "loading" },
      }),
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: `Expand dispatch detail for ${PROVIDER_DISPATCH_ID}`,
      }),
    );
    expect(screen.getByText("Loading dispatch detail…")).toBeTruthy();

    const renderState = (state: FactorySessionDispatchDetailViewState) =>
      rerender(
        <FactorySessionDetailPanel
          detailState={detailState}
          inspectionState={createInspectionState(
            { [PROVIDER_DISPATCH_ID]: state },
            {
              onRetryDispatchDetail: (dispatchID) => {
                retryIDs.push(dispatchID);
              },
            },
          )}
          lifecycleControl={createLifecycleControl()}
          sessionID={SESSION_ID}
        />,
      );

    renderState({ status: "not-found" });
    expect(
      screen.getByText(
        `Dispatch detail for ${PROVIDER_DISPATCH_ID} is no longer available.`,
      ),
    ).toBeTruthy();

    renderState({
      message: "Dispatch detail service unavailable.",
      status: "error",
    });
    expect(
      screen.getByText("Dispatch detail service unavailable."),
    ).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Retry loading dispatch detail" }),
    );
    expect(retryIDs).toEqual([PROVIDER_DISPATCH_ID]);
  });

  it("preserves event replay unavailable and artifact failure outcomes from typed states", () => {
    const { rerender } = renderPanel(
      createInspectionState(
        { [PROVIDER_DISPATCH_ID]: { status: "idle" } },
        {
          eventReplay: {
            message: "Durable replay is unavailable.",
            status: "unavailable",
          },
          artifactDrilldowns: {
            [ARTIFACT_ID]: {
              failure: {
                kind: "network",
                message: "Artifact service unavailable.",
              },
              status: "error",
            },
          },
        },
      ),
    );

    fireEvent.click(
      screen.getByRole("button", { name: `View artifact ${ARTIFACT_ID}` }),
    );
    expect(screen.getByText("Artifact service unavailable.")).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Expand Factory Event replay" }),
    );
    expect(screen.getByText("Durable replay is unavailable.")).toBeTruthy();

    rerender(
      <FactorySessionDetailPanel
        detailState={detailState}
        inspectionState={createInspectionState(
          { [PROVIDER_DISPATCH_ID]: { status: "idle" } },
          {
            eventReplay: { status: "loading" },
            artifactDrilldowns: {
              [ARTIFACT_ID]: { status: "loading" },
            },
          },
        )}
        lifecycleControl={createLifecycleControl()}
        sessionID={SESSION_ID}
      />,
    );
    expect(screen.getByText("Loading artifact detail…")).toBeTruthy();
    expect(
      screen.getByText("Loading durable Factory Event replay…"),
    ).toBeTruthy();
  });
});
