import { describe, expect, it } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  normalizeStreamDerivedCacheIdentity,
  streamDerivedCacheKeyPrefix,
  streamDerivedCheckpointStorageKey,
} from "./stream-derived-cache-identity";

const IDENTITY_FIXTURE = {
  backendScopeID: "backend-scope-a",
  factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
  logicalSessionKeyID: "logical-default",
  streamGenerationID: "2026-06-26T00:00:00Z",
} as const;

describe("stream-derived cache identity", () => {
  it("normalizes valid identity and rejects alias session ids", () => {
    expect(normalizeStreamDerivedCacheIdentity(IDENTITY_FIXTURE)).toEqual(
      IDENTITY_FIXTURE,
    );
    expect(
      normalizeStreamDerivedCacheIdentity({
        ...IDENTITY_FIXTURE,
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
      }),
    ).toBeNull();
    expect(normalizeStreamDerivedCacheIdentity(null)).toBeNull();
  });

  it("builds cache key prefixes and checkpoint storage keys", () => {
    expect(streamDerivedCacheKeyPrefix(IDENTITY_FIXTURE)).toEqual([
      "backend-scope-a",
      "a1b2c3d4-e5f6-4789-a012-3456789abcde",
      "2026-06-26T00:00:00Z",
    ]);
    expect(streamDerivedCheckpointStorageKey(IDENTITY_FIXTURE)).toBe(
      "backend-scope-a::a1b2c3d4-e5f6-4789-a012-3456789abcde::2026-06-26T00:00:00Z",
    );
  });
});
