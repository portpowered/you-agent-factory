import { describe, expect, it } from "vitest";
import * as publicDiagnostics from "../../public/session-persistence-diagnostics";
import {
  createSessionPersistenceCorrelationToken,
  readSessionPersistenceDiagnosticRecords,
  recordSessionPersistenceDiagnostic,
  resetSessionPersistenceDiagnosticRecords,
  SESSION_PERSISTENCE_DIAGNOSTIC_CAPACITY,
  SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME,
  sessionPersistenceDiagnostic,
} from "./diagnostics";

describe("session-persistence-diagnostics", () => {
  it("exposes all nine outcomes with one deterministic recovery action", () => {
    expect(SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME).toEqual({
      checkpoint_hit: "reuse_checkpoint",
      checkpoint_miss: "replay_without_cursor",
      restore_succeeded: "resume_from_checkpoint",
      identity_rejected: "discard_rejected_checkpoint",
      logical_remap: "switch_to_resolved_session",
      durable_write_succeeded: "none_required",
      durable_write_failed: "retain_last_committed_checkpoint",
      stale_cursor: "invalidate_reconnect_cursor",
      cursor_free_replay_fallback: "replay_without_cursor",
    });
    for (const [outcome, recoveryAction] of Object.entries(
      SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME,
    )) {
      expect(
        sessionPersistenceDiagnostic(
          outcome as keyof typeof SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME,
          createSessionPersistenceCorrelationToken("factory-session-a"),
        ),
      ).toMatchObject({ outcome, recoveryAction });
    }
  });

  it("re-exports the bounded contract through the narrow public barrel", () => {
    expect(publicDiagnostics.SESSION_PERSISTENCE_DIAGNOSTIC_CAPACITY).toBe(100);
    expect(publicDiagnostics.recordSessionPersistenceDiagnostic).toBe(
      recordSessionPersistenceDiagnostic,
    );
  });
});

describe("session-persistence diagnostic retention", () => {
  it("keeps the newest 100 records in emission order", () => {
    resetSessionPersistenceDiagnosticRecords();
    for (
      let index = 0;
      index <= SESSION_PERSISTENCE_DIAGNOSTIC_CAPACITY;
      index += 1
    ) {
      recordSessionPersistenceDiagnostic({
        outcome: "checkpoint_miss",
        correlationToken: createSessionPersistenceCorrelationToken(
          `factory-session-${index}`,
        ),
      });
    }

    const records = readSessionPersistenceDiagnosticRecords();
    expect(records).toHaveLength(100);
    expect(records[0].correlationToken).toBe(
      createSessionPersistenceCorrelationToken("factory-session-1"),
    );
    expect(records.at(-1)?.correlationToken).toBe(
      createSessionPersistenceCorrelationToken("factory-session-100"),
    );
  });

  it("returns defensive snapshots and does not retain input mutations", () => {
    resetSessionPersistenceDiagnosticRecords();
    const input: Record<string, unknown> = {
      outcome: "identity_rejected",
      correlationToken:
        createSessionPersistenceCorrelationToken("factory-session-a"),
      detail: "factory_session_mismatch",
    };
    expect(recordSessionPersistenceDiagnostic(input)).toBe(true);
    input.detail = "backend_scope_mismatch";

    const snapshot = readSessionPersistenceDiagnosticRecords();
    snapshot[0].detail = "stream_generation_mismatch";
    snapshot.push(
      sessionPersistenceDiagnostic(
        "checkpoint_hit",
        createSessionPersistenceCorrelationToken("factory-session-b"),
      ),
    );

    expect(readSessionPersistenceDiagnosticRecords()).toEqual([
      {
        outcome: "identity_rejected",
        recoveryAction: "discard_rejected_checkpoint",
        correlationToken:
          createSessionPersistenceCorrelationToken("factory-session-a"),
        detail: "factory_session_mismatch",
      },
    ]);
    resetSessionPersistenceDiagnosticRecords();
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);
  });

  it("creates stable non-reversible correlation tokens", () => {
    const rawIdentity = "customer-session-secret";
    const first = createSessionPersistenceCorrelationToken(rawIdentity);
    expect(first).toBe(createSessionPersistenceCorrelationToken(rawIdentity));
    expect(first).not.toBe(
      createSessionPersistenceCorrelationToken("another-session"),
    );
    expect(first).toMatch(/^spc_[a-f0-9]{64}$/);
    expect(first).not.toContain(rawIdentity);
    expect(createSessionPersistenceCorrelationToken("abc")).toBe(
      "spc_ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
    );
    expect(() => createSessionPersistenceCorrelationToken(" ")).toThrow(
      TypeError,
    );
  });

  it("rejects sensitive payload-shaped and caller-action input", () => {
    resetSessionPersistenceDiagnosticRecords();
    const token = createSessionPersistenceCorrelationToken("factory-session-a");
    const rejected = [
      { outcome: "checkpoint_hit", correlationToken: token, event: {} },
      { outcome: "checkpoint_hit", correlationToken: token, work: {} },
      { outcome: "checkpoint_hit", correlationToken: token, cursor: "raw" },
      {
        outcome: "checkpoint_hit",
        correlationToken: token,
        error: new Error(),
      },
      {
        outcome: "checkpoint_hit",
        correlationToken: token,
        storageKey: "/tmp/key",
      },
      {
        outcome: "checkpoint_hit",
        correlationToken: token,
        recoveryAction: "none_required",
      },
      { outcome: "not_an_outcome", correlationToken: token },
      { outcome: "checkpoint_hit", correlationToken: "raw-session-id" },
      Object.defineProperty({}, "outcome", {
        enumerable: true,
        get: () => {
          throw new Error("sensitive getter");
        },
      }),
    ];
    for (const input of rejected) {
      expect(recordSessionPersistenceDiagnostic(input)).toBe(false);
    }
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);
  });
});
