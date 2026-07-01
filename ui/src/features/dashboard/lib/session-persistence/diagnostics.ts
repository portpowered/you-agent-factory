export type SessionPersistenceInvalidationReason =
  | "cursor_stale"
  | "session_not_found"
  | "session_remapped"
  | "stream_generation_changed"
  | "backend_scope_changed"
  | "user_cleared_sessions";

export type SessionPersistenceRecoveryAction =
  | "clear_checkpoint"
  | "clear_stream_derived_state"
  | "replay_without_cursor"
  | "show_explicit_recovery"
  | "reuse_checkpoint";

export interface SessionPersistenceIdentityScope {
  backendScopeID?: string;
  logicalSessionKeyID?: string;
  factorySessionID?: string;
  streamGenerationID?: string;
}

export interface SessionPersistenceInvalidationDiagnostic {
  reason: SessionPersistenceInvalidationReason;
  recoveryAction: SessionPersistenceRecoveryAction;
  scope: SessionPersistenceIdentityScope;
  requestedSessionID?: string;
  previousScope?: SessionPersistenceIdentityScope;
}

const invalidationRecords: SessionPersistenceInvalidationDiagnostic[] = [];

export function resetSessionPersistenceInvalidationRecords(): void {
  invalidationRecords.length = 0;
}

export function readSessionPersistenceInvalidationRecords(): SessionPersistenceInvalidationDiagnostic[] {
  return [...invalidationRecords];
}

export function recordSessionPersistenceInvalidation(
  diagnostic: SessionPersistenceInvalidationDiagnostic,
): void {
  invalidationRecords.push(structuredClone(diagnostic));
}

export function classifyCheckpointIdentityMismatch(
  previous: SessionPersistenceIdentityScope,
  current: SessionPersistenceIdentityScope,
): SessionPersistenceInvalidationReason | null {
  const normalizedPrevious = normalizeIdentityScope(previous);
  const normalizedCurrent = normalizeIdentityScope(current);
  if (scopesEqual(normalizedPrevious, normalizedCurrent)) {
    return null;
  }
  if (
    normalizedPrevious.backendScopeID &&
    normalizedCurrent.backendScopeID &&
    normalizedPrevious.backendScopeID !== normalizedCurrent.backendScopeID
  ) {
    return "backend_scope_changed";
  }
  if (
    normalizedPrevious.factorySessionID &&
    normalizedCurrent.factorySessionID &&
    normalizedPrevious.factorySessionID !== normalizedCurrent.factorySessionID
  ) {
    return "session_remapped";
  }
  if (
    normalizedPrevious.streamGenerationID &&
    normalizedCurrent.streamGenerationID &&
    normalizedPrevious.streamGenerationID !== normalizedCurrent.streamGenerationID
  ) {
    return "stream_generation_changed";
  }
  return "stream_generation_changed";
}

export function recoveryActionForIdentityMismatch(
  reason: SessionPersistenceInvalidationReason,
): SessionPersistenceRecoveryAction {
  switch (reason) {
    case "backend_scope_changed":
    case "session_remapped":
    case "stream_generation_changed":
      return "clear_stream_derived_state";
    default:
      return "clear_checkpoint";
  }
}

export function identityMismatchDiagnostic(
  previous: SessionPersistenceIdentityScope,
  current: SessionPersistenceIdentityScope,
  requestedSessionID: string,
): SessionPersistenceInvalidationDiagnostic | null {
  const reason = classifyCheckpointIdentityMismatch(previous, current);
  if (!reason) {
    return null;
  }
  return {
    reason,
    recoveryAction: recoveryActionForIdentityMismatch(reason),
    scope: normalizeIdentityScope(current),
    previousScope: normalizeIdentityScope(previous),
    requestedSessionID,
  };
}

export function silentReplayRecoveryDiagnostic(
  scope: SessionPersistenceIdentityScope,
  requestedSessionID: string,
): SessionPersistenceInvalidationDiagnostic {
  return {
    reason: "cursor_stale",
    recoveryAction: "replay_without_cursor",
    scope: normalizeIdentityScope(scope),
    requestedSessionID,
  };
}

export function userClearedSessionsDiagnostic(
  scope: SessionPersistenceIdentityScope,
  requestedSessionID: string,
): SessionPersistenceInvalidationDiagnostic {
  return {
    reason: "user_cleared_sessions",
    recoveryAction: "clear_checkpoint",
    scope: normalizeIdentityScope(scope),
    requestedSessionID,
  };
}

function normalizeIdentityScope(
  scope: SessionPersistenceIdentityScope,
): SessionPersistenceIdentityScope {
  return {
    backendScopeID: scope.backendScopeID?.trim() || undefined,
    logicalSessionKeyID: scope.logicalSessionKeyID?.trim() || undefined,
    factorySessionID: scope.factorySessionID?.trim() || undefined,
    streamGenerationID: scope.streamGenerationID?.trim() || undefined,
  };
}

function scopesEqual(
  left: SessionPersistenceIdentityScope,
  right: SessionPersistenceIdentityScope,
): boolean {
  return (
    left.backendScopeID === right.backendScopeID &&
    left.logicalSessionKeyID === right.logicalSessionKeyID &&
    left.factorySessionID === right.factorySessionID &&
    left.streamGenerationID === right.streamGenerationID
  );
}
