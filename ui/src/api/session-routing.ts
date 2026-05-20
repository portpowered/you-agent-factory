export const DEFAULT_FACTORY_SESSION_ID = "~default";

export function isDefaultFactorySessionID(sessionID: string | null | undefined): boolean {
  return sessionID == null || sessionID === "" || sessionID === DEFAULT_FACTORY_SESSION_ID;
}

export function factorySessionScopedPath(
  legacyPath: string,
  sessionID: string | null | undefined,
): string {
  if (isDefaultFactorySessionID(sessionID)) {
    return legacyPath;
  }

  const normalizedPath = legacyPath.startsWith("/") ? legacyPath : `/${legacyPath}`;
  return `/factories/${encodeURIComponent(sessionID ?? "")}${normalizedPath}`;
}
