import { describe, expect, it } from "vitest";

import type { FactorySessionSyncPreflightResponse } from "../../../../api/factory-sessions/sync-preflight";
import { FactorySessionSyncPreflightReasonCode } from "../../../../api/generated/openapi";
import {
  checkpointSyncIdentityFromPreflight,
  isNonRecoverableSyncPreflightReasonCode,
  resolveDashboardSyncPreflight,
  shouldClearCheckpointAfterPreflight,
  shouldRemapDashboardSession,
  syncPreflightIdentityHintsFromCheckpoint,
} from "./dashboard-session-sync-preflight";

function buildPreflightResponse(
  overrides: Partial<FactorySessionSyncPreflightResponse> = {},
): FactorySessionSyncPreflightResponse {
  return {
    backendScopeId: "backend-scope-a",
    checkpointReusable: true,
    factorySessionId: "session-live-001",
    logicalSessionKeyId: "lsk-default-folder",
    reasonCode: FactorySessionSyncPreflightReasonCode.ok,
    reconnectCursor: {
      afterEventId: "event-7",
      afterSequence: 7,
      provided: true,
      validForStreamGeneration: true,
    },
    requestedSessionId: "session-live-001",
    streamGenerationId: "2026-06-26T00:00:00Z",
    ...overrides,
  };
}

describe("dashboard-session-sync-preflight resolution", () => {
  it("resolves same-stream resume with a validated reconnect cursor", () => {
    const resolution = resolveDashboardSyncPreflight(buildPreflightResponse());

    expect(resolution).toEqual({
      checkpointReusable: true,
      kind: "resume",
      reconnectCursor: {
        afterEventId: "event-7",
        afterSequence: 7,
      },
      requestedSessionId: "session-live-001",
      resolvedSessionId: "session-live-001",
      streamIdentity: {
        backendScopeID: "backend-scope-a",
        factorySessionID: "session-live-001",
        logicalSessionKeyID: "lsk-default-folder",
        streamGenerationID: "2026-06-26T00:00:00Z",
      },
    });
  });

  it("drops reconnect cursors for logical session remap outcomes", () => {
    const resolution = resolveDashboardSyncPreflight(
      buildPreflightResponse({
        checkpointReusable: false,
        factorySessionId: "session-remapped-002",
        reasonCode: FactorySessionSyncPreflightReasonCode.logical_session_remap,
        reconnectCursor: {
          afterEventId: "event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
        requestedSessionId: "session-stale-001",
      }),
    );

    expect(resolution).toMatchObject({
      checkpointReusable: false,
      kind: "resume",
      reconnectCursor: undefined,
      requestedSessionId: "session-stale-001",
      resolvedSessionId: "session-remapped-002",
    });
    expect(
      shouldRemapDashboardSession(
        buildPreflightResponse({
          checkpointReusable: false,
          factorySessionId: "session-remapped-002",
          reasonCode:
            FactorySessionSyncPreflightReasonCode.logical_session_remap,
          requestedSessionId: "session-stale-001",
        }),
        "session-stale-001",
      ),
    ).toBe(true);
    expect(
      shouldClearCheckpointAfterPreflight(
        buildPreflightResponse({
          reasonCode:
            FactorySessionSyncPreflightReasonCode.logical_session_remap,
        }),
      ),
    ).toBe(true);
  });

  it("fails closed when resumable outcomes omit identity fields", () => {
    const resolution = resolveDashboardSyncPreflight(
      buildPreflightResponse({
        backendScopeId: undefined,
        factorySessionId: undefined,
        logicalSessionKeyId: undefined,
        streamGenerationId: undefined,
      }),
    );

    expect(resolution).toEqual({
      kind: "non-recoverable",
      recovery: {
        reasonCode:
          FactorySessionSyncPreflightReasonCode.invalid_target_reference,
        requestedSessionId: "session-live-001",
      },
    });
  });
});

describe("dashboard-session-sync-preflight non-recoverable resolution", () => {
  it("returns non-recoverable outcomes for unresolved logical targets", () => {
    const resolution = resolveDashboardSyncPreflight(
      buildPreflightResponse({
        checkpointReusable: false,
        reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: false,
        },
        requestedSessionId: "session-missing",
      }),
    );

    expect(resolution).toEqual({
      kind: "non-recoverable",
      recovery: {
        reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
        requestedSessionId: "session-missing",
      },
    });
  });

  it("treats unknown reason codes as non-recoverable", () => {
    const resolution = resolveDashboardSyncPreflight(
      buildPreflightResponse({
        reasonCode: "logical_session_unresolved",
      }),
    );

    expect(resolution).toEqual({
      kind: "non-recoverable",
      recovery: {
        reasonCode: "logical_session_unresolved",
        requestedSessionId: "session-live-001",
      },
    });
  });

  it("resumes cursor_stale outcomes without a reconnect cursor", () => {
    const resolution = resolveDashboardSyncPreflight(
      buildPreflightResponse({
        checkpointReusable: false,
        reasonCode: FactorySessionSyncPreflightReasonCode.cursor_stale,
        reconnectCursor: {
          afterEventId: "event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
      }),
    );

    expect(resolution).toMatchObject({
      checkpointReusable: false,
      kind: "resume",
      reconnectCursor: undefined,
      resolvedSessionId: "session-live-001",
    });
  });
});

describe("dashboard-session-sync-preflight helpers", () => {
  it("extracts logical identity hints from persisted checkpoint sync identity", () => {
    expect(
      syncPreflightIdentityHintsFromCheckpoint(
        {
          backendScopeId: "backend-scope-a",
          factorySessionId: "session-stale-001",
          logicalSessionKeyId: "lsk-named-target",
          streamGenerationId: "2026-06-26T00:00:00Z",
        },
        null,
      ),
    ).toEqual({
      backendScopeId: "backend-scope-a",
      logicalSessionKeyId: "lsk-named-target",
    });
  });

  it("falls back to stream identity hints when checkpoint sync identity is absent", () => {
    expect(
      syncPreflightIdentityHintsFromCheckpoint(undefined, {
        backendScopeID: "backend-scope-b",
        factorySessionID: "session-live",
        logicalSessionKeyID: "lsk-folder",
        streamGenerationID: "generation-2",
      }),
    ).toEqual({
      backendScopeId: "backend-scope-b",
      logicalSessionKeyId: "lsk-folder",
    });
  });

  it("prefers checkpoint sync identity logical key over envelope stream identity", () => {
    expect(
      syncPreflightIdentityHintsFromCheckpoint(
        {
          backendScopeId: "backend-scope-a",
          factorySessionId: "session-stale-001",
          logicalSessionKeyId: "lsk-checkpoint",
          streamGenerationId: "generation-1",
        },
        {
          backendScopeID: "backend-scope-b",
          factorySessionID: "session-live",
          logicalSessionKeyID: "lsk-envelope",
          streamGenerationID: "generation-2",
        },
      ),
    ).toEqual({
      backendScopeId: "backend-scope-a",
      logicalSessionKeyId: "lsk-checkpoint",
    });
  });

  it("maps preflight stream identity into checkpoint sync identity", () => {
    expect(
      checkpointSyncIdentityFromPreflight({
        backendScopeID: "backend-scope-a",
        factorySessionID: "session-live-001",
        logicalSessionKeyID: "lsk-default",
        streamGenerationID: "generation-1",
      }),
    ).toEqual({
      backendScopeId: "backend-scope-a",
      factorySessionId: "session-live-001",
      logicalSessionKeyId: "lsk-default",
      streamGenerationId: "generation-1",
    });
  });

  it("classifies non-recoverable sync-preflight reason codes", () => {
    expect(
      isNonRecoverableSyncPreflightReasonCode(
        FactorySessionSyncPreflightReasonCode.session_not_found,
      ),
    ).toBe(true);
    expect(
      isNonRecoverableSyncPreflightReasonCode(
        FactorySessionSyncPreflightReasonCode.invalid_target_reference,
      ),
    ).toBe(true);
    expect(
      isNonRecoverableSyncPreflightReasonCode(
        FactorySessionSyncPreflightReasonCode.ok,
      ),
    ).toBe(false);
  });
});
