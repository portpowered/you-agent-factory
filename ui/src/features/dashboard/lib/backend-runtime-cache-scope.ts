export const UNSAFE_BACKEND_RUNTIME_CACHE_SCOPE = "__unsafe__" as const;

export function normalizeBackendRuntimeCacheScope(
  backendScopeID: string | null | undefined,
): string | null {
  const trimmed = backendScopeID?.trim() ?? "";
  return trimmed === "" ? null : trimmed;
}

export function isBackendRuntimeCacheScopeReady(
  backendScopeID: string | null | undefined,
): boolean {
  return normalizeBackendRuntimeCacheScope(backendScopeID) != null;
}

export function backendRuntimeCacheScopeKey(
  backendScopeID: string | null | undefined,
): string {
  return (
    normalizeBackendRuntimeCacheScope(backendScopeID) ??
    UNSAFE_BACKEND_RUNTIME_CACHE_SCOPE
  );
}

export function scopedRuntimeQueryKey(
  prefix: readonly string[],
  sessionID: string | null | undefined,
  backendScopeID: string | null | undefined,
): readonly string[] {
  return [
    ...prefix,
    backendRuntimeCacheScopeKey(backendScopeID),
    sessionID ?? "",
  ];
}

export function shouldResetDashboardRuntimeScopedState({
  previousBackendScope,
  backendRuntimeCacheScope,
}: {
  previousBackendScope: string | null;
  backendRuntimeCacheScope: string | null;
}): boolean {
  if (previousBackendScope === null) {
    return false;
  }
  if (!isBackendRuntimeCacheScopeReady(backendRuntimeCacheScope)) {
    return backendRuntimeCacheScope === null;
  }
  return previousBackendScope !== backendRuntimeCacheScope;
}
