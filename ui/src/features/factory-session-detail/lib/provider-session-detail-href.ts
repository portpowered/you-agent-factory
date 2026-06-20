import type { components } from "../../../api/generated/openapi";
import { toProviderSessionDetailRef } from "../../../api/provider-session-details";

export function buildProviderSessionDetailHref(
  providerSessionRef: components["schemas"]["LoadableProviderSessionRef"],
): string | null {
  const detailRef = toProviderSessionDetailRef(providerSessionRef);
  if (detailRef === null) {
    return null;
  }

  const params = new URLSearchParams({
    id: detailRef.id,
    kind: detailRef.kind,
    provider: detailRef.provider,
  });
  return `/provider-sessions/detail?${params.toString()}`;
}

export function formatProviderSessionRefLabel(
  providerSessionRef: components["schemas"]["LoadableProviderSessionRef"],
): string {
  return `${providerSessionRef.provider} / ${providerSessionRef.kind} / ${providerSessionRef.id}`;
}
