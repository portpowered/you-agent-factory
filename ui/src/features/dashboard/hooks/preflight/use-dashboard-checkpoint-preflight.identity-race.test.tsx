import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as factorySessionsAPI from "../../../../api/factory-sessions";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import { createMaterializedWorkOutcomeState } from "../../../work-outcome/public/materializer";
import { useDashboardCheckpointPreflight } from "./use-dashboard-checkpoint-preflight";

vi.mock("../../session/dashboard-session-provider", () => ({
  useRemapDashboardSelectedSession: () => vi.fn(),
}));

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useDashboardCheckpointPreflight exact deletion cancellation", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("preserves persisted and runtime state when deletion is superseded", async () => {
    const sessionID = "44444444-4444-4444-8444-444444444444";
    const streamIdentity = {
      backendScopeID: "backend-delete-race",
      factorySessionID: sessionID,
      logicalSessionKeyID: "logical-delete-race",
      streamGenerationID: "generation-delete-race",
    };
    const persistedRecord = {
      checkpoint: {
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: {},
        selectedTick: 11,
      },
      schemaVersion: 4,
      storageKey: Object.values(streamIdentity).join("::"),
      streamIdentity,
    };
    const { controls, indexedDB, records } =
      createControlledIndexedDBTestDouble<typeof persistedRecord>();
    records.set(persistedRecord.storageKey, persistedRecord);
    vi.stubGlobal("indexedDB", indexedDB);
    vi.spyOn(
      factorySessionsAPI,
      "getFactorySessionSyncPreflight",
    ).mockResolvedValue({
      backendScopeId: persistedRecord.streamIdentity.backendScopeID,
      checkpointReusable: false,
      factorySessionId: sessionID,
      logicalSessionKeyId: persistedRecord.streamIdentity.logicalSessionKeyID,
      reasonCode: FactorySessionSyncPreflightReasonCode.ok,
      reconnectCursor: {
        provided: false,
        validForStreamGeneration: false,
      },
      requestedSessionId: sessionID,
      streamGenerationId: "replacement-generation",
    });

    const queryClient = new QueryClient();
    queryClient.setQueryData(["session-race", sessionID], "cache-a");
    const removeQueries = vi.spyOn(queryClient, "removeQueries");
    const hook = renderHook(
      ({ disabled }) =>
        useDashboardCheckpointPreflight({
          checkpointHydrationKey: `${sessionID}::0`,
          checkpointsDisabled: disabled,
          rawSessionID: sessionID,
          refreshToken: 0,
          restoreCheckpoint: vi.fn(),
        }),
      {
        initialProps: { disabled: false },
        wrapper: createWrapper(queryClient),
      },
    );

    await flushPromiseContinuations();
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["getAll"]);
    controls.succeed("getAll");
    await waitFor(() => {
      expect(controls.pendingOperations()).toEqual(["open"]);
    });
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["delete"]);

    hook.rerender({ disabled: true });
    controls.succeed("delete");
    await flushPromiseContinuations();

    expect(records.get(persistedRecord.storageKey)).toEqual(persistedRecord);
    expect(queryClient.getQueryData(["session-race", sessionID])).toBe(
      "cache-a",
    );
    expect(removeQueries).not.toHaveBeenCalled();
  });
});
