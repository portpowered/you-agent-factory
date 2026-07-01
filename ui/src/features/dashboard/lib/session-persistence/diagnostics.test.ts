import { describe, expect, it } from "vitest";

import {
  classifyCheckpointIdentityMismatch,
  identityMismatchDiagnostic,
  readSessionPersistenceInvalidationRecords,
  recordSessionPersistenceInvalidation,
  resetSessionPersistenceInvalidationRecords,
  silentReplayRecoveryDiagnostic,
} from "./diagnostics";
import { userClearedSessionsDiagnostic } from "../../public/session-persistence-diagnostics";

describe("session-persistence-diagnostics", () => {
  it("re-exports diagnostics through the narrow public barrel", () => {
    expect(
      userClearedSessionsDiagnostic(
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
        "session-a",
      ),
    ).toEqual({
      reason: "user_cleared_sessions",
      recoveryAction: "clear_checkpoint",
      requestedSessionID: "session-a",
      scope: {
        backendScopeID: "backend-a",
        factorySessionID: "session-a",
        streamGenerationID: "stream-a",
      },
    });
  });

  it("records and resets invalidation diagnostics for test inspection", () => {
    resetSessionPersistenceInvalidationRecords();
    recordSessionPersistenceInvalidation(
      silentReplayRecoveryDiagnostic(
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
        "session-a",
      ),
    );

    expect(readSessionPersistenceInvalidationRecords()).toEqual([
      {
        reason: "cursor_stale",
        recoveryAction: "replay_without_cursor",
        requestedSessionID: "session-a",
        scope: {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
      },
    ]);

    resetSessionPersistenceInvalidationRecords();
    expect(readSessionPersistenceInvalidationRecords()).toEqual([]);
  });

  it("classifies backend scope changes separately from stream generation changes", () => {
    expect(
      classifyCheckpointIdentityMismatch(
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
        {
          backendScopeID: "backend-b",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
      ),
    ).toBe("backend_scope_changed");

    expect(
      classifyCheckpointIdentityMismatch(
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-b",
        },
      ),
    ).toBe("stream_generation_changed");
  });

  it("builds silent replay diagnostics without a reconnect cursor", () => {
    expect(
      silentReplayRecoveryDiagnostic(
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
        "session-a",
      ),
    ).toEqual({
      reason: "cursor_stale",
      recoveryAction: "replay_without_cursor",
      requestedSessionID: "session-a",
      scope: {
        backendScopeID: "backend-a",
        factorySessionID: "session-a",
        streamGenerationID: "stream-a",
      },
    });
  });

  it("records previous and current scope for identity mismatch diagnostics", () => {
    expect(
      identityMismatchDiagnostic(
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-a",
          streamGenerationID: "stream-a",
        },
        {
          backendScopeID: "backend-a",
          factorySessionID: "session-b",
          streamGenerationID: "stream-b",
        },
        "~default",
      ),
    ).toEqual({
      reason: "session_remapped",
      recoveryAction: "clear_stream_derived_state",
      requestedSessionID: "~default",
      previousScope: {
        backendScopeID: "backend-a",
        factorySessionID: "session-a",
        streamGenerationID: "stream-a",
      },
      scope: {
        backendScopeID: "backend-a",
        factorySessionID: "session-b",
        streamGenerationID: "stream-b",
      },
    });
  });
});
