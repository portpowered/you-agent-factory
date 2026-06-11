import { describe, expect, it } from "vitest";

import {
  createFixtureBackedDurableSessionInspectionAdapter,
  type FixtureBackedDurableSessionInspectionAdapter,
} from "./adapters";
import {
  DURABLE_SESSION_INSPECTION_SCENARIO_IDS,
  DURABLE_SESSION_INSPECTION_SCENARIO_PURPOSES,
  type DurableSessionInspectionScenarioPurpose,
  durableSessionInspectionScenarioID,
} from "./fixture-catalog";
import {
  createInspectionFixtureScenarioIndex,
  loadJavaScriptInspectionFixtureScenarios,
} from "./fixture-scenarios";

const adapter = createFixtureBackedDurableSessionInspectionAdapter();
const index = createInspectionFixtureScenarioIndex();

async function assertScenarioPurposeSignals(
  adapterInstance: FixtureBackedDurableSessionInspectionAdapter,
  purpose: DurableSessionInspectionScenarioPurpose,
): Promise<void> {
  const scenarioId = durableSessionInspectionScenarioID(purpose);
  const scenario = index.byScenarioId.get(scenarioId);
  expect(scenario, `missing scenario ${scenarioId}`).toBeDefined();
  if (!scenario?.session) {
    throw new Error(`scenario ${scenarioId} is missing session data`);
  }

  const sessionId = scenario.session.sessionId;
  const detail = await adapterInstance.getSession(sessionId);
  expect(detail.status).toBe("success");
  if (detail.status !== "success") {
    throw new Error(`expected detail success for ${scenarioId}`);
  }

  switch (purpose) {
    case "running":
      expect(detail.data.status).toBe("RUNNING");
      expect(detail.data.progress?.inFlightDispatches).toBeGreaterThan(0);
      return;
    case "completed":
      expect(detail.data.status).toBe("SUCCEEDED");
      expect(detail.data.resultSummary?.resultStatus).toBe("FINAL");
      return;
    case "failed-recoverable":
      expect(detail.data.status).toBe("INTERRUPTED");
      expect(detail.data.staleLease).toBe(true);
      return;
    case "result-not-ready": {
      const result = await adapterInstance.getResult(sessionId);
      expect(result.status).toBe("success");
      if (result.status !== "success") {
        throw new Error(`expected result for ${scenarioId}`);
      }
      expect(result.data.resultStatus).toBe("NOT_READY");
      return;
    }
    case "result-available": {
      const result = await adapterInstance.getResult(sessionId);
      expect(result.status).toBe("success");
      if (result.status !== "success") {
        throw new Error(`expected result for ${scenarioId}`);
      }
      expect(result.data.resultStatus).toBe("FINAL");
      expect(result.data.primaryResult?.length).toBeGreaterThan(0);
      return;
    }
    case "dispatch-list": {
      const dispatches = await adapterInstance.listDispatches(sessionId);
      expect(dispatches.status).toBe("success");
      if (dispatches.status !== "success") {
        throw new Error(`expected dispatches for ${scenarioId}`);
      }
      expect(dispatches.data.dispatches.length).toBeGreaterThan(0);
      return;
    }
    case "artifact-list": {
      const artifacts = await adapterInstance.listArtifacts(sessionId);
      expect(artifacts.status).toBe("success");
      if (artifacts.status !== "success") {
        throw new Error(`expected artifacts for ${scenarioId}`);
      }
      expect(artifacts.data.artifacts.length).toBeGreaterThan(0);
      return;
    }
    case "artifact-detail": {
      const artifactId = scenario.artifacts[0]?.id;
      if (!artifactId) {
        throw new Error(`expected artifact id for ${scenarioId}`);
      }
      const artifact = await adapterInstance.getArtifact(sessionId, artifactId);
      expect(artifact.status).toBe("success");
      if (artifact.status !== "success") {
        throw new Error(`expected artifact detail for ${scenarioId}`);
      }
      expect(artifact.data.id).toBe(artifactId);
      return;
    }
    case "empty-dispatches": {
      const dispatches = await adapterInstance.listDispatches(sessionId);
      expect(dispatches.status).toBe("empty");
      return;
    }
    case "empty-artifacts": {
      const artifacts = await adapterInstance.listArtifacts(sessionId);
      expect(artifacts.status).toBe("empty");
      return;
    }
    default:
      throw new Error(`unhandled purpose ${purpose satisfies never}`);
  }
}

describe("fixture-backed durable session inspection adapters", () => {
  it("loads JavaScript contract fixture scenarios plus the builtin recoverable session", () => {
    expect(loadJavaScriptInspectionFixtureScenarios().length).toBeGreaterThan(
      5,
    );
    expect(index.byScenarioId.has("javascript-interrupted-recoverable")).toBe(
      true,
    );
    expect(index.byScenarioId.has("javascript-running-n-dispatch")).toBe(true);
  });

  it("exposes explicit loading, empty, error, and success outcomes", async () => {
    await expect(
      adapter.listSessions({ simulate: "loading" }),
    ).resolves.toEqual({ status: "loading" });
    await expect(adapter.listSessions({ simulate: "empty" })).resolves.toEqual({
      status: "empty",
    });
    await expect(
      adapter.listSessions({
        errorMessage: "fixture list failed",
        simulate: "error",
      }),
    ).resolves.toEqual({
      code: undefined,
      message: "fixture list failed",
      status: "error",
    });

    const listOutcome = await adapter.listSessions();
    expect(listOutcome.status).toBe("success");
    if (listOutcome.status !== "success") {
      throw new Error("expected success list outcome");
    }
    expect(listOutcome.data.scope).toBe("persisted");
    expect(listOutcome.data.sessions.length).toBeGreaterThan(0);
    expect(
      listOutcome.data.sessions.every(
        (session) => session.orchestratorKind === "JAVASCRIPT",
      ),
    ).toBe(true);
  });

  it.each(
    DURABLE_SESSION_INSPECTION_SCENARIO_PURPOSES,
  )("covers inspection scenario %s with expected durable session signals", async (purpose) => {
    await assertScenarioPurposeSignals(adapter, purpose);
  });

  it("returns list rows with action availability and artifact counts when present", async () => {
    const scenario = index.byScenarioId.get(
      DURABLE_SESSION_INSPECTION_SCENARIO_IDS["failed-recoverable"],
    );
    expect(scenario).toBeDefined();

    const listOutcome = await adapter.listSessions();
    expect(listOutcome.status).toBe("success");
    if (listOutcome.status !== "success") {
      throw new Error("expected list success");
    }

    const row = listOutcome.data.sessions.find(
      (session) => session.sessionId === scenario?.session?.sessionId,
    );
    expect(row).toBeDefined();
    expect(row?.recoverable).toBe(true);
    expect(row?.actions?.canResume).toBe(true);
    expect(row?.artifactCount).toBe(0);
  });

  it("returns typed not-found errors for unknown session and child resources", async () => {
    await expect(adapter.getSession("dur-sess-unknown")).resolves.toEqual({
      code: "NOT_FOUND",
      message:
        "Durable factory session dur-sess-unknown was not found in fixtures.",
      status: "error",
    });
    await expect(adapter.listDispatches("dur-sess-unknown")).resolves.toEqual({
      code: "NOT_FOUND",
      message:
        "Durable factory session dur-sess-unknown was not found in fixtures.",
      status: "error",
    });
    const runningScenario = index.byScenarioId.get(
      DURABLE_SESSION_INSPECTION_SCENARIO_IDS.running,
    );
    const runningSessionId = runningScenario?.session?.sessionId;
    if (!runningSessionId) {
      throw new Error("expected running scenario session id");
    }
    await expect(
      adapter.getDispatch(runningSessionId, "disp-missing"),
    ).resolves.toEqual({
      code: "NOT_FOUND",
      message: expect.stringContaining("was not found"),
      status: "error",
    });
  });

  it("projects dispatch detail from fixture dispatchDetail rows", async () => {
    const scenario = index.byScenarioId.get(
      DURABLE_SESSION_INSPECTION_SCENARIO_IDS["dispatch-list"],
    );
    const dispatchId = scenario?.dispatchDetail?.id;
    const dispatchSessionId = scenario?.session?.sessionId;
    if (!dispatchId || !dispatchSessionId) {
      throw new Error("expected dispatch detail fixture");
    }

    const dispatch = await adapter.getDispatch(dispatchSessionId, dispatchId);
    expect(dispatch.status).toBe("success");
    if (dispatch.status !== "success") {
      throw new Error("expected dispatch detail");
    }
    expect(dispatch.data.sessionId).toBe(scenario?.session?.sessionId);
    expect(dispatch.data.javascript?.taskLabel).toBe("verify-release");
  });
});
