import { describe, expect, it } from "bun:test";
import { render, screen } from "@testing-library/react";

import {
  FactoryOrchestratorKind,
  FactorySessionDurableLifecycleStatus,
  FactorySessionStatus,
} from "../../../api/generated/openapi";
import type {
  FactorySessionDetailData,
  FactorySessionDetailViewState,
} from "../hooks/use-factory-session-detail";
import { FactorySessionDetailPanel } from "./factory-session-detail-panel";

const SESSION_ID = "session-presentation";

const successfulDetailData: FactorySessionDetailData = {
  durableLifecycleStatus:
    FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded,
  durableReadModel: {
    budgets: { maxAgents: 2 },
    effectivePolicy: { policyHash: "sha256:policy-presentation" },
    latestCheckpoint: {
      id: "checkpoint-1",
      label: "Plan",
      phase: "verify",
    },
    orchestratorKind: FactoryOrchestratorKind.PETRI,
    phase: "verify",
    phaseSummaries: [
      {
        completedDispatchCount: 1,
        dispatchCount: 1,
        label: "Plan",
        phase: "plan",
      },
      { dispatchCount: 1, phase: "verify" },
    ],
    progress: {
      completedDispatches: 1,
      totalDispatches: 1,
    },
    resolvedSource: {
      kind: "FACTORY_ID",
      sourceRef: "factory/alpha",
    },
    resultSummary: { resultStatus: "PARTIAL" },
    sessionId: SESSION_ID,
    status:
      FactorySessionDurableLifecycleStatus.FactorySessionDurableLifecycleStatusSucceeded,
    usage: { resources: [] },
  },
  session: {
    factoryDir: "/workspace/root/alpha",
    folderPath: "/workspace/root",
    id: SESSION_ID,
    isDefault: false,
    project: "alpha",
    runtime: {
      lifecycle: {
        startedAt: "2026-06-08T14:00:00Z",
        updatedAt: "2026-06-08T14:05:00Z",
      },
      orchestratorKind: FactoryOrchestratorKind.PETRI,
      petri: { enabledTransitions: [], marking: [] },
      progress: {
        categories: {
          failed: 0,
          initial: 0,
          processing: 0,
          terminal: 0,
        },
        factoryState: "RUNNING",
        inFlightCount: 0,
        totalTokens: 0,
      },
      status: FactorySessionStatus.IDLE,
      usage: { resources: [] },
    },
    target: { kind: "named", name: "alpha" },
  },
};

function renderPanel(
  detailState: FactorySessionDetailViewState,
  locale?: string,
) {
  return render(
    <FactorySessionDetailPanel
      detailState={detailState}
      locale={locale}
      sessionID={SESSION_ID}
    />,
  );
}

describe("FactorySessionDetailPanel presentation states", () => {
  it("renders loading, unavailable, and request-error states from explicit inputs", () => {
    const { rerender } = renderPanel({ status: "loading" });
    expect(screen.getByRole("status").textContent).toContain(
      "Loading factory session runtime…",
    );

    rerender(
      <FactorySessionDetailPanel
        detailState={{ status: "not-found" }}
        sessionID={SESSION_ID}
      />,
    );
    expect(screen.getByRole("status").textContent).toContain(
      "This factory session is no longer available.",
    );

    rerender(
      <FactorySessionDetailPanel
        detailState={{
          message: "Factory session request failed.",
          status: "error",
        }}
        sessionID={SESSION_ID}
      />,
    );
    expect(screen.getByRole("alert").textContent).toContain(
      "Factory session request failed.",
    );
  });

  it("renders a successful session and durable summary from typed detail data", () => {
    renderPanel({ data: successfulDetailData, status: "success" });

    expect(screen.getByRole("heading", { name: "Runtime" })).toBeTruthy();
    expect(screen.getByText(SESSION_ID)).toBeTruthy();
    expect(screen.getByText("Succeeded")).toBeTruthy();
    expect(screen.getByText("factory/alpha")).toBeTruthy();
    expect(screen.getByText("checkpoint-1 (Plan) · verify")).toBeTruthy();
    expect(screen.getByText("verify — current")).toBeTruthy();
    expect(screen.getByText("Unavailable")).toBeTruthy();
  });

  it("keeps loading and unavailable copy localized when state is supplied directly", () => {
    const { rerender } = renderPanel({ status: "loading" }, "zh-CN");
    expect(screen.getByRole("status").textContent).toContain(
      "正在加载工厂会话运行时…",
    );

    rerender(
      <FactorySessionDetailPanel
        detailState={{ status: "not-found" }}
        locale="zh-CN"
        sessionID={SESSION_ID}
      />,
    );
    expect(screen.getByRole("status").textContent).toContain(
      "此工厂会话已不可用。",
    );
  });
});
