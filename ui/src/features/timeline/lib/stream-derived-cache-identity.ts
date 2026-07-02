import { isDefaultFactorySessionID } from "../../../api/session-routing";

export interface StreamDerivedCacheIdentity {
  backendScopeID: string;
  factorySessionID: string;
  logicalSessionKeyID: string;
  streamGenerationID: string;
}

export function normalizeStreamDerivedCacheIdentity(
  identity: Partial<StreamDerivedCacheIdentity> | null | undefined,
): StreamDerivedCacheIdentity | null {
  if (!identity) {
    return null;
  }
  const backendScopeID = identity.backendScopeID?.trim() ?? "";
  const factorySessionID = identity.factorySessionID?.trim() ?? "";
  const logicalSessionKeyID = identity.logicalSessionKeyID?.trim() ?? "";
  const streamGenerationID = identity.streamGenerationID?.trim() ?? "";
  if (
    backendScopeID === "" ||
    logicalSessionKeyID === "" ||
    streamGenerationID === "" ||
    factorySessionID === "" ||
    isDefaultFactorySessionID(factorySessionID)
  ) {
    return null;
  }
  return {
    backendScopeID,
    factorySessionID,
    logicalSessionKeyID,
    streamGenerationID,
  };
}

export function streamDerivedCacheKeyPrefix(
  identity: StreamDerivedCacheIdentity,
): readonly [string, string, string] {
  return [
    identity.backendScopeID,
    identity.factorySessionID,
    identity.streamGenerationID,
  ];
}

export function streamDerivedCheckpointStorageKey(
  identity: StreamDerivedCacheIdentity,
): string {
  return streamDerivedCacheKeyPrefix(identity).join("::");
}
