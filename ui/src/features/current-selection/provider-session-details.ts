import type { DashboardProviderSessionAttempt } from "../../api/dashboard/types";

export interface LoadableProviderSessionRef {
  dispatchID: string;
  id: string;
  kind: string;
  provider: string;
}

const LOADABLE_PROVIDER_SESSION_ID_PATTERN = /^[A-Za-z0-9_-]+$/;

export function getLoadableProviderSessionRef(
  attempt: Pick<DashboardProviderSessionAttempt, "dispatch_id" | "provider_session">,
): LoadableProviderSessionRef | null {
  const session = attempt.provider_session;
  const provider = normalizeProviderSessionPart(session?.provider);
  const kind = normalizeProviderSessionPart(session?.kind);
  const id = normalizeProviderSessionID(session?.id);

  if (provider !== "codex" || kind !== "session_id" || !id) {
    return null;
  }

  return {
    dispatchID: attempt.dispatch_id,
    id,
    kind,
    provider,
  };
}

export function providerSessionSelectionKey(
  session: Pick<LoadableProviderSessionRef, "dispatchID" | "id" | "kind" | "provider">,
): string {
  return `${session.dispatchID}:${session.provider}:${session.kind}:${session.id}`;
}

function normalizeProviderSessionID(value: string | undefined): string | null {
  const trimmed = value?.trim();
  return trimmed && LOADABLE_PROVIDER_SESSION_ID_PATTERN.test(trimmed)
    ? trimmed
    : null;
}

function normalizeProviderSessionPart(value: string | undefined): string | null {
  const trimmed = value?.trim().toLowerCase();
  return trimmed && trimmed.length > 0 ? trimmed : null;
}
