export const SESSION_PERSISTENCE_DIAGNOSTIC_CAPACITY = 100;

export const SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME = {
  checkpoint_hit: "reuse_checkpoint",
  checkpoint_miss: "replay_without_cursor",
  restore_succeeded: "resume_from_checkpoint",
  identity_rejected: "discard_rejected_checkpoint",
  logical_remap: "switch_to_resolved_session",
  durable_write_succeeded: "none_required",
  durable_write_failed: "retain_last_committed_checkpoint",
  stale_cursor: "invalidate_reconnect_cursor",
  cursor_free_replay_fallback: "replay_without_cursor",
} as const;

export type SessionPersistenceDiagnosticOutcome =
  keyof typeof SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME;

export type SessionPersistenceRecoveryAction =
  (typeof SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME)[SessionPersistenceDiagnosticOutcome];

export type SessionPersistenceDiagnosticDetail =
  | "backend_scope_mismatch"
  | "factory_session_mismatch"
  | "logical_session_mismatch"
  | "stream_generation_mismatch";

export interface SessionPersistenceDiagnostic {
  outcome: SessionPersistenceDiagnosticOutcome;
  recoveryAction: SessionPersistenceRecoveryAction;
  correlationToken: string;
  detail?: SessionPersistenceDiagnosticDetail;
}

export interface SessionPersistenceIdentityScope {
  backendScopeID?: string;
  logicalSessionKeyID?: string;
  factorySessionID?: string;
  streamGenerationID?: string;
}

export type SessionPersistenceInvalidationReason =
  | "backend_scope_changed"
  | "session_remapped"
  | "stream_generation_changed";

const diagnosticRecords: SessionPersistenceDiagnostic[] = [];
const diagnosticOutcomes = new Set<string>(
  Object.keys(SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME),
);
const diagnosticDetails = new Set<string>([
  "backend_scope_mismatch",
  "factory_session_mismatch",
  "logical_session_mismatch",
  "stream_generation_mismatch",
]);
const correlationTokenPattern = /^spc_[a-f0-9]{64}$/;

export function resetSessionPersistenceDiagnosticRecords(): void {
  diagnosticRecords.length = 0;
}

export function readSessionPersistenceDiagnosticRecords(): SessionPersistenceDiagnostic[] {
  return diagnosticRecords.map((record) => ({ ...record }));
}

/**
 * Best-effort process-local capture. Unknown or payload-shaped input is ignored
 * so this boundary cannot accidentally retain customer data.
 */
export function recordSessionPersistenceDiagnostic(input: unknown): boolean {
  let diagnostic: SessionPersistenceDiagnostic | null;
  try {
    diagnostic = parseDiagnosticInput(input);
  } catch {
    return false;
  }
  if (!diagnostic) {
    return false;
  }
  if (diagnosticRecords.length === SESSION_PERSISTENCE_DIAGNOSTIC_CAPACITY) {
    diagnosticRecords.shift();
  }
  diagnosticRecords.push(diagnostic);
  return true;
}

export function sessionPersistenceDiagnostic(
  outcome: SessionPersistenceDiagnosticOutcome,
  correlationToken: string,
  detail?: SessionPersistenceDiagnosticDetail,
): SessionPersistenceDiagnostic {
  return {
    outcome,
    recoveryAction: SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME[outcome],
    correlationToken,
    ...(detail ? { detail } : {}),
  };
}

export function createSessionPersistenceCorrelationToken(
  approvedIdentity: string,
): string {
  const normalizedIdentity = approvedIdentity.trim();
  if (!normalizedIdentity || normalizedIdentity.length > 4096) {
    throw new TypeError(
      "Correlation identity must contain 1 to 4096 characters",
    );
  }
  return `spc_${sha256(normalizedIdentity)}`;
}

export function correlationTokenForIdentityScope(
  scope: SessionPersistenceIdentityScope,
): string {
  const normalized = normalizeIdentityScope(scope);
  return createSessionPersistenceCorrelationToken(
    [
      normalized.backendScopeID ?? "",
      normalized.logicalSessionKeyID ?? "",
      normalized.factorySessionID ?? "",
      normalized.streamGenerationID ?? "",
    ].join("\u001f"),
  );
}

export function classifyCheckpointIdentityMismatch(
  previous: SessionPersistenceIdentityScope,
  current: SessionPersistenceIdentityScope,
): SessionPersistenceInvalidationReason | null {
  const detail = classifyIdentityMismatchDetail(previous, current);
  switch (detail) {
    case "backend_scope_mismatch":
      return "backend_scope_changed";
    case "factory_session_mismatch":
      return "session_remapped";
    case "logical_session_mismatch":
    case "stream_generation_mismatch":
      return "stream_generation_changed";
    default:
      return null;
  }
}

export function classifyCheckpointIdentityMismatchDetail(
  previous: SessionPersistenceIdentityScope,
  current: SessionPersistenceIdentityScope,
): SessionPersistenceDiagnosticDetail | null {
  return classifyIdentityMismatchDetail(previous, current);
}

export function identityMismatchDiagnostic(
  previous: SessionPersistenceIdentityScope,
  current: SessionPersistenceIdentityScope,
  _requestedSessionID: string,
): SessionPersistenceDiagnostic | null {
  const detail = classifyIdentityMismatchDetail(previous, current);
  if (!detail) {
    return null;
  }
  return sessionPersistenceDiagnostic(
    "identity_rejected",
    correlationTokenForIdentityScope(current),
    detail,
  );
}

export function silentReplayRecoveryDiagnostic(
  scope: SessionPersistenceIdentityScope,
  _requestedSessionID: string,
): SessionPersistenceDiagnostic {
  return sessionPersistenceDiagnostic(
    "stale_cursor",
    correlationTokenForIdentityScope(scope),
  );
}

function parseDiagnosticInput(
  input: unknown,
): SessionPersistenceDiagnostic | null {
  if (!isPlainObject(input)) {
    return null;
  }
  const keys = Object.keys(input);
  if (
    keys.some(
      (key) =>
        !["outcome", "recoveryAction", "correlationToken", "detail"].includes(
          key,
        ),
    ) ||
    typeof input.outcome !== "string" ||
    !diagnosticOutcomes.has(input.outcome) ||
    typeof input.correlationToken !== "string" ||
    !correlationTokenPattern.test(input.correlationToken) ||
    (input.recoveryAction !== undefined &&
      input.recoveryAction !==
        SESSION_PERSISTENCE_RECOVERY_ACTION_BY_OUTCOME[
          input.outcome as SessionPersistenceDiagnosticOutcome
        ]) ||
    (input.detail !== undefined &&
      (typeof input.detail !== "string" ||
        !diagnosticDetails.has(input.detail)))
  ) {
    return null;
  }
  return sessionPersistenceDiagnostic(
    input.outcome as SessionPersistenceDiagnosticOutcome,
    input.correlationToken,
    input.detail as SessionPersistenceDiagnosticDetail | undefined,
  );
}

function isPlainObject(input: unknown): input is Record<string, unknown> {
  if (typeof input !== "object" || input === null) {
    return false;
  }
  const prototype = Object.getPrototypeOf(input);
  return prototype === Object.prototype || prototype === null;
}

function classifyIdentityMismatchDetail(
  previous: SessionPersistenceIdentityScope,
  current: SessionPersistenceIdentityScope,
): SessionPersistenceDiagnosticDetail | null {
  const left = normalizeIdentityScope(previous);
  const right = normalizeIdentityScope(current);
  const comparisons: Array<
    [keyof SessionPersistenceIdentityScope, SessionPersistenceDiagnosticDetail]
  > = [
    ["backendScopeID", "backend_scope_mismatch"],
    ["factorySessionID", "factory_session_mismatch"],
    ["logicalSessionKeyID", "logical_session_mismatch"],
    ["streamGenerationID", "stream_generation_mismatch"],
  ];
  for (const [field, detail] of comparisons) {
    if (left[field] !== right[field]) {
      return detail;
    }
  }
  return null;
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

function sha256(value: string): string {
  const bytes = new TextEncoder().encode(value);
  const paddedLength = Math.ceil((bytes.length + 9) / 64) * 64;
  const padded = new Uint8Array(paddedLength);
  padded.set(bytes);
  padded[bytes.length] = 0x80;
  const view = new DataView(padded.buffer);
  const bitLength = bytes.length * 8;
  view.setUint32(paddedLength - 8, Math.floor(bitLength / 0x100000000));
  view.setUint32(paddedLength - 4, bitLength >>> 0);

  const hash = [...SHA256_INITIAL_HASH];
  const words = new Uint32Array(64);
  for (let offset = 0; offset < paddedLength; offset += 64) {
    for (let index = 0; index < 16; index += 1) {
      words[index] = view.getUint32(offset + index * 4);
    }
    for (let index = 16; index < 64; index += 1) {
      const first = words[index - 15];
      const second = words[index - 2];
      const sigma0 =
        rotateRight(first, 7) ^ rotateRight(first, 18) ^ (first >>> 3);
      const sigma1 =
        rotateRight(second, 17) ^ rotateRight(second, 19) ^ (second >>> 10);
      words[index] =
        (words[index - 16] + sigma0 + words[index - 7] + sigma1) >>> 0;
    }
    compressSha256Block(hash, words);
  }
  return hash.map((word) => word.toString(16).padStart(8, "0")).join("");
}

function compressSha256Block(hash: number[], words: Uint32Array): void {
  let [a, b, c, d, e, f, g, h] = hash;
  for (let index = 0; index < 64; index += 1) {
    const sigma1 = rotateRight(e, 6) ^ rotateRight(e, 11) ^ rotateRight(e, 25);
    const choice = (e & f) ^ (~e & g);
    const temp1 =
      (h + sigma1 + choice + SHA256_CONSTANTS[index] + words[index]) >>> 0;
    const sigma0 = rotateRight(a, 2) ^ rotateRight(a, 13) ^ rotateRight(a, 22);
    const majority = (a & b) ^ (a & c) ^ (b & c);
    const temp2 = (sigma0 + majority) >>> 0;
    h = g;
    g = f;
    f = e;
    e = (d + temp1) >>> 0;
    d = c;
    c = b;
    b = a;
    a = (temp1 + temp2) >>> 0;
  }
  const state = [a, b, c, d, e, f, g, h];
  for (let index = 0; index < hash.length; index += 1) {
    hash[index] = (hash[index] + state[index]) >>> 0;
  }
}

function rotateRight(value: number, places: number): number {
  return (value >>> places) | (value << (32 - places));
}

const SHA256_INITIAL_HASH = [
  0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c,
  0x1f83d9ab, 0x5be0cd19,
] as const;

const SHA256_CONSTANTS = [
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1,
  0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
  0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786,
  0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147,
  0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
  0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
  0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a,
  0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
  0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
] as const;

// Compatibility aliases for existing consumers while owner migrations proceed.
export const resetSessionPersistenceInvalidationRecords =
  resetSessionPersistenceDiagnosticRecords;
export const readSessionPersistenceInvalidationRecords =
  readSessionPersistenceDiagnosticRecords;
export const recordSessionPersistenceInvalidation =
  recordSessionPersistenceDiagnostic;
