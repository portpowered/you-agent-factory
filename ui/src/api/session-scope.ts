import {
  currentFactorySessionPath,
  DEFAULT_FACTORY_SESSION_ID,
  factorySessionEventsPath,
  factorySessionWorkPath,
  isDefaultFactorySessionID,
} from "./session-routing";

export interface SessionScope {
  eventsPath: string;
  factoryPath: string;
  isDefault: boolean;
  isPaused: boolean;
  rawSessionID: string | null;
  sessionID: string;
  workPath: string;
}

export function buildSessionScope(
  selectedSessionID: string | null,
  pausedSessionIDs: readonly string[],
): SessionScope {
  const isDefault = isDefaultFactorySessionID(selectedSessionID);
  const sessionID = isDefault
    ? DEFAULT_FACTORY_SESSION_ID
    : (selectedSessionID ?? DEFAULT_FACTORY_SESSION_ID);

  return {
    sessionID,
    rawSessionID: selectedSessionID,
    isDefault,
    factoryPath: currentFactorySessionPath(selectedSessionID),
    workPath: factorySessionWorkPath(selectedSessionID),
    eventsPath: factorySessionEventsPath(selectedSessionID),
    isPaused:
      selectedSessionID != null && pausedSessionIDs.includes(selectedSessionID),
  };
}
