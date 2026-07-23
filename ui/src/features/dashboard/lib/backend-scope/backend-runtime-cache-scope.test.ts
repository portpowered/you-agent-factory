import { describe, expect, it } from "vitest";

import {
  backendRuntimeCacheScopeKey,
  isBackendRuntimeCacheScopeReady,
  normalizeBackendRuntimeCacheScope,
  shouldResetDashboardRuntimeScopedState,
  UNSAFE_BACKEND_RUNTIME_CACHE_SCOPE,
} from "./backend-runtime-cache-scope";

describe("backend runtime cache scope", () => {
  it("normalizes blank backend scope values to unsafe cache reuse", () => {
    expect(normalizeBackendRuntimeCacheScope("  local-abc  ")).toBe(
      "local-abc",
    );
    expect(normalizeBackendRuntimeCacheScope("")).toBeNull();
    expect(normalizeBackendRuntimeCacheScope("   ")).toBeNull();
    expect(normalizeBackendRuntimeCacheScope(undefined)).toBeNull();
    expect(isBackendRuntimeCacheScopeReady("local-abc")).toBe(true);
    expect(isBackendRuntimeCacheScopeReady("")).toBe(false);
    expect(backendRuntimeCacheScopeKey(null)).toBe(
      UNSAFE_BACKEND_RUNTIME_CACHE_SCOPE,
    );
  });

  it("resets runtime state only after a proven backend scope changes", () => {
    expect(
      shouldResetDashboardRuntimeScopedState({
        previousBackendScope: null,
        backendRuntimeCacheScope: "backend-a",
      }),
    ).toBe(false);
    expect(
      shouldResetDashboardRuntimeScopedState({
        previousBackendScope: "backend-a",
        backendRuntimeCacheScope: "backend-b",
      }),
    ).toBe(true);
    expect(
      shouldResetDashboardRuntimeScopedState({
        previousBackendScope: "backend-a",
        backendRuntimeCacheScope: null,
      }),
    ).toBe(true);
    expect(
      shouldResetDashboardRuntimeScopedState({
        previousBackendScope: "backend-a",
        backendRuntimeCacheScope: "backend-a",
      }),
    ).toBe(false);
  });
});
