import { FactoryOrchestratorKind } from "../generated/openapi";
import {
  normalizeFactorySessionGetResponse,
  runtimeStatusFromDurableLifecycle,
  scriptStatusFromDurableLifecycle,
} from "./normalize-session-get";

describe("normalizeFactorySessionGetResponse", () => {
  it("normalizes dispatch-free live factory sessions without copying them", () => {
    const liveSession = {
      factoryDir: "/workspace/root/beta",
      folderPath: "/workspace/root",
      id: "session-beta",
      isDefault: false,
      project: "beta",
      runtime: {
        lifecycle: {
          startedAt: "2026-06-08T14:00:00Z",
          updatedAt: "2026-06-08T14:05:00Z",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        progress: {
          categories: {},
          factoryState: "RUNNING",
          inFlightCount: 0,
          totalTokens: 0,
        },
        status: "IDLE",
        usage: { resources: [] },
      },
      target: { kind: "named", name: "beta" },
    };

    expect(normalizeFactorySessionGetResponse(liveSession)).toEqual({
      session: liveSession,
    });
    expect(normalizeFactorySessionGetResponse(liveSession).session).toBe(
      liveSession,
    );
  });

  it("discards legacy embedded dispatches without mutating the live response", () => {
    const liveSession = {
      factoryDir: "/workspace/root/beta",
      folderPath: "/workspace/root",
      id: "session-beta",
      isDefault: false,
      project: "beta",
      runtime: {
        dispatches: [{ id: "dispatch-legacy" }],
        lifecycle: {
          startedAt: "2026-06-08T14:00:00Z",
          updatedAt: "2026-06-08T14:05:00Z",
        },
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        progress: {
          categories: {},
          factoryState: "RUNNING",
          inFlightCount: 1,
          totalTokens: 12,
        },
        status: "ACTIVE",
        usage: { resources: [] },
      },
      target: { kind: "named", name: "beta" },
    };

    const normalized = normalizeFactorySessionGetResponse(liveSession);

    expect(normalized.session.runtime).toEqual({
      lifecycle: liveSession.runtime.lifecycle,
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      progress: liveSession.runtime.progress,
      status: "ACTIVE",
      usage: { resources: [] },
    });
    expect("dispatches" in normalized.session.runtime).toBe(false);
    expect(liveSession.runtime.dispatches).toEqual([{ id: "dispatch-legacy" }]);
  });

  it("maps durable JavaScript session reads into shared FactorySession runtime shape", () => {
    const durableRead = {
      budgets: { maxAgents: 3 },
      dialect: "you-workflow-v1",
      lifecycle: {
        startedAt: "2026-06-08T14:00:00Z",
        updatedAt: "2026-06-08T14:05:00Z",
      },
      orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
      phase: "verify",
      phaseSummaries: [
        { completedDispatchCount: 1, dispatchCount: 1, phase: "plan" },
        { dispatchCount: 2, phase: "verify" },
      ],
      progress: {
        completedDispatches: 1,
        failedDispatches: 0,
        inFlightDispatches: 1,
        totalDispatches: 3,
      },
      resolvedSource: {
        kind: "WORKFLOW_NAME",
        sourceRef: "workflow/release-train",
        sourceHash: "sha256:js-workflow-release-train",
      },
      sessionId: "dur-sess-js-run-n-001",
      sourceHash: "sha256:js-workflow-release-train",
      status: "RUNNING",
      usage: { resources: [] },
    } as const;
    const normalized = normalizeFactorySessionGetResponse(durableRead);

    expect(normalized.durableLifecycleStatus).toBe("RUNNING");
    expect(normalized.durableReadModel).toBe(durableRead);
    expect(normalized.session.id).toBe("dur-sess-js-run-n-001");
    expect(normalized.session.runtime.orchestratorKind).toBe(
      FactoryOrchestratorKind.JAVASCRIPT,
    );
    expect(normalized.session.runtime.status).toBe("ACTIVE");
    expect(normalized.session.runtime.javascript).toEqual({
      childDispatchCounts: {
        completed: 1,
        queued: 1,
        running: 1,
      },
      phase: "verify",
      phases: ["plan", "verify"],
      scriptStatus: "RUNNING",
    });
  });
});

describe("durable lifecycle status mapping", () => {
  it("maps durable lifecycle statuses to shared runtime and script statuses", () => {
    expect(runtimeStatusFromDurableLifecycle("RUNNING")).toBe("ACTIVE");
    expect(runtimeStatusFromDurableLifecycle("PAUSED")).toBe("IDLE");
    expect(runtimeStatusFromDurableLifecycle("SUCCEEDED")).toBe("FINISHED");

    expect(scriptStatusFromDurableLifecycle("RUNNING")).toBe("RUNNING");
    expect(scriptStatusFromDurableLifecycle("PAUSED")).toBe("PAUSED");
    expect(scriptStatusFromDurableLifecycle("FAILED")).toBe("FAILED");
    expect(scriptStatusFromDurableLifecycle("SUCCEEDED")).toBe("FINISHED");
  });
});
