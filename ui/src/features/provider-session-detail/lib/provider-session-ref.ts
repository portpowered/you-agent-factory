import type { DashboardProviderSessionAttempt } from "../../../api/dashboard/types";
import {
  toProviderSessionDetailRef,
  type ProviderSessionDetailRef,
} from "../../../api/provider-session-details";

export interface LoadableProviderSessionRef extends ProviderSessionDetailRef {
  dispatchID: string;
}

export function getLoadableProviderSessionRef(
  attempt: Pick<DashboardProviderSessionAttempt, "dispatch_id" | "provider_session">,
): LoadableProviderSessionRef | null {
  const request = toProviderSessionDetailRef({
    id: attempt.provider_session?.id,
    kind: attempt.provider_session?.kind,
    provider: attempt.provider_session?.provider,
  });
  if (request === null) {
    return null;
  }

  return {
    dispatchID: attempt.dispatch_id,
    ...request,
  };
}

export function providerSessionSelectionKey(
  session: Pick<LoadableProviderSessionRef, "dispatchID" | "id" | "kind" | "provider">,
): string {
  return `${session.dispatchID}:${session.provider}:${session.kind}:${session.id}`;
}
