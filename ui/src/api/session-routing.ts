export const DEFAULT_FACTORY_SESSION_ID = "~default";

export function isDefaultFactorySessionID(sessionID: string | null | undefined): boolean {
  return sessionID == null || sessionID === "" || sessionID === DEFAULT_FACTORY_SESSION_ID;
}

export function currentFactorySessionPath(sessionID: string | null | undefined): string {
  const normalizedSessionID: string = isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
  return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}/factory`;
}

export function factorySessionScopedPath(
  legacyPath: string,
  sessionID: string | null | undefined,
): string {
  const normalizedPath = legacyPath.startsWith("/") ? legacyPath : `/${legacyPath}`;
  if (normalizedPath === "/factory/~current") {
    return currentFactorySessionPath(sessionID);
  }
  const normalizedSessionID: string = isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
  return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}${normalizedPath}`;
}
