import { vi } from "vitest";

import { FactoryOrchestratorKind } from "../../../../api/generated/openapi";
import { BASELINE_SESSION_ID } from "./factory-session-detail-panel.baseline-fixtures";
import { jsonResponse } from "./factory-session-detail-panel.test-helpers";

export const NOT_FOUND_SESSION_ID = "session-missing";
export const PETRI_SESSION_ID = "~default";

export function mockSessionNotFoundFetch() {
  vi.mocked(globalThis.fetch).mockResolvedValue(
    new Response(
      JSON.stringify({
        code: "NOT_FOUND",
        message: "Factory session missing.",
      }),
      {
        headers: {
          "Content-Type": "application/json",
        },
        status: 404,
        statusText: "Not Found",
      },
    ),
  );
}

export function mockSessionApiErrorFetch() {
  vi.mocked(globalThis.fetch).mockResolvedValue(
    new Response(JSON.stringify({ code: "INTERNAL_ERROR", message: "boom" }), {
      headers: { "Content-Type": "application/json" },
      status: 500,
    }),
  );
}

export function mockPausedLifecycleFetch() {
  vi.mocked(globalThis.fetch).mockResolvedValue(
    jsonResponse({
      factoryDir: "/workspace/root/beta",
      folderPath: "/workspace/root",
      id: BASELINE_SESSION_ID,
      isDefault: false,
      project: "beta",
      runtime: {
        lifecycle: {
          startedAt: "2026-06-08T14:00:00Z",
          updatedAt: "2026-06-08T14:05:00Z",
        },
        lifecycleControlStatus: "PAUSED",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        progress: {
          categories: {},
          factoryState: "PAUSED",
          inFlightCount: 0,
          totalTokens: 0,
        },
        status: "ACTIVE",
        usage: { resources: [] },
      },
      target: { kind: "named", name: "beta" },
    }),
  );
}

export function mockRunningLifecycleFetch() {
  vi.mocked(globalThis.fetch).mockResolvedValue(
    jsonResponse({
      factoryDir: "/workspace/root/beta",
      folderPath: "/workspace/root",
      id: BASELINE_SESSION_ID,
      isDefault: false,
      project: "beta",
      runtime: {
        lifecycle: {
          startedAt: "2026-06-08T14:00:00Z",
          updatedAt: "2026-06-08T14:10:00Z",
        },
        lifecycleControlStatus: "RUNNING",
        orchestratorKind: FactoryOrchestratorKind.JAVASCRIPT,
        progress: {
          categories: {},
          factoryState: "RUNNING",
          inFlightCount: 1,
          totalTokens: 0,
        },
        status: "ACTIVE",
        usage: { resources: [] },
      },
      target: { kind: "named", name: "beta" },
    }),
  );
}

export function mockPetriRuntimeDetailFetch() {
  vi.mocked(globalThis.fetch).mockResolvedValue(
    jsonResponse({
      factoryDir: "/workspace/root",
      folderPath: "/workspace/root",
      id: PETRI_SESSION_ID,
      isDefault: true,
      project: "root",
      runtime: {
        lifecycle: {
          startedAt: "2026-06-08T14:00:00Z",
          updatedAt: "2026-06-08T14:05:00Z",
        },
        orchestratorKind: FactoryOrchestratorKind.PETRI,
        petri: {
          enabledTransitions: [
            {
              transitionId: "tr-process",
              workerType: "worker-a",
            },
          ],
          marking: [{ id: "tok-1" }],
        },
        progress: {
          categories: {},
          factoryState: "RUNNING",
          inFlightCount: 0,
          totalTokens: 1,
        },
        status: "IDLE",
        usage: { resources: [] },
      },
      target: { kind: "default" },
    }),
  );
}
