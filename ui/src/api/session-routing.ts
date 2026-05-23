export const DEFAULT_FACTORY_SESSION_ID = "~default";

export function isDefaultFactorySessionID(sessionID: string | null | undefined): boolean {
  return sessionID == null || sessionID === "" || sessionID === DEFAULT_FACTORY_SESSION_ID;
}

export function currentFactorySessionPath(sessionID: string | null | undefined): string {
  const normalizedSessionID: string = isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}/factory`;
}

export function currentFactoryWorkstationPath(
  workstationName: string,
  sessionID: string | null | undefined,
  suffix: "prompt-template-contract" | "prompt-template-validation",
): string {
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `${currentFactorySessionPath(sessionID)}/workstations/${encodeURIComponent(workstationName)}/${suffix}`;
}

export function factorySessionScopedPath(
  path: string,
  sessionID: string | null | undefined,
): string {
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  const normalizedSessionID: string = isDefaultFactorySessionID(sessionID)
    ? DEFAULT_FACTORY_SESSION_ID
    : (sessionID ?? DEFAULT_FACTORY_SESSION_ID);
  return `/factory-sessions/${encodeURIComponent(normalizedSessionID)}${normalizedPath}`;
}
