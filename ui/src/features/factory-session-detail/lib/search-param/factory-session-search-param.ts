export const FACTORY_SESSION_ID_SEARCH_PARAM = "factorySessionId";

export function readFactorySessionIDSearchParam(
  locationSearch: string | null | undefined,
): string | null {
  const rawValue =
    new URLSearchParams(locationSearch ?? "").get(
      FACTORY_SESSION_ID_SEARCH_PARAM,
    ) ?? "";
  const sessionID = rawValue.trim();
  return sessionID.length > 0 ? sessionID : null;
}
