export const DEFAULT_FACTORY_SESSION_ID = "~default";

export function isDefaultFactorySessionID(sessionID: string | null | undefined): boolean {
  return sessionID == null || sessionID === "" || sessionID === DEFAULT_FACTORY_SESSION_ID;
}

export function factorySessionScopedPath(
  legacyPath: string,
  sessionID: string | null | undefined,
): string {
  const normalizedPath = legacyPath.startsWith("/") ? legacyPath : `/${legacyPath}`;
  const normalizedSessionID: string = isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
  if (normalizedPath === "/factory/~current") {
    return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}/factory`;
  }
  if (normalizedPath === "/factory/~current/editable-definition") {
    return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}/factory/editable-definition`;
  }
  return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}${normalizedPath}`;
}
