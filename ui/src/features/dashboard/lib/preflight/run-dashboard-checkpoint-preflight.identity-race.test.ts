import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as factorySessionsAPI from "../../../../api/factory-sessions";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import {
  createControlledIndexedDBTestDouble,
  flushPromiseContinuations,
} from "../../../../testing/controlled-indexeddb-test-utils";
import * as timelinePublic from "../../../timeline/public";
import { runDashboardCheckpointPreflight } from "./run-dashboard-checkpoint-preflight";

describe("runDashboardCheckpointPreflight identity mismatch cancellation", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("does not delete persisted or runtime state after becoming obsolete", async () => {
    const sessionID = "session-a";
    const persistedRecord = {
      checkpoint: { replayState: {}, selectedTick: 11 },
      schemaVersion: 3,
      storageKey: `checkpoint-${sessionID}`,
      streamIdentity: {
        backendScopeID: `backend-${sessionID}`,
        factorySessionID: sessionID,
        logicalSessionKeyID: `logical-${sessionID}`,
        streamGenerationID: `generation-${sessionID}`,
      },
    };
    const { controls, indexedDB, records } =
      createControlledIndexedDBTestDouble<typeof persistedRecord>();
    records.set(persistedRecord.storageKey, persistedRecord);
    vi.stubGlobal("indexedDB", indexedDB);
    vi.spyOn(timelinePublic, "readTimelineCheckpoint").mockResolvedValue(null);
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
    const abortController = new AbortController();
    let isCurrent = true;
    const preflight = runDashboardCheckpointPreflight({
      isCurrent: () => isCurrent,
      onRemapSessionID: vi.fn(),
      onStreamOffline: vi.fn(),
      queryClient,
      rawSessionID: sessionID,
      restoreCheckpoint: vi.fn(),
      signal: abortController.signal,
    });

    await flushPromiseContinuations();
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["open"]);
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["getAll"]);
    controls.succeed("getAll");
    await flushPromiseContinuations();
    await flushPromiseContinuations();
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["open"]);
    controls.succeed("open");
    await flushPromiseContinuations();
    expect(controls.pendingOperations()).toEqual(["delete"]);

    isCurrent = false;
    abortController.abort();
    controls.succeed("delete");
    await preflight;

    expect(records.get(persistedRecord.storageKey)).toEqual(persistedRecord);
    expect(queryClient.getQueryData(["session-race", sessionID])).toBe(
      "cache-a",
    );
    expect(removeQueries).not.toHaveBeenCalled();
  });
});
