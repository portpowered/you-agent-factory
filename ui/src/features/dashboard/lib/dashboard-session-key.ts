export function dashboardSessionKey(
  sessionID: string | null,
  refreshToken: number,
): string | null {
  return sessionID == null ? null : `${sessionID}::${refreshToken}`;
}

export function sessionIDFromDashboardSessionKey(
  sessionKey: string | null,
): string | null {
  if (sessionKey == null) {
    return null;
  }
  const separatorIndex = sessionKey.lastIndexOf("::");
  return separatorIndex === -1
    ? sessionKey
    : sessionKey.slice(0, separatorIndex);
}

export function shouldResumeFromPersistedCheckpoint({
  previousSessionKey,
  refreshToken,
  sessionID,
}: {
  previousSessionKey: string | null;
  refreshToken: number;
  sessionID: string | null;
}): boolean {
  if (refreshToken === 0) {
    return true;
  }
  if (sessionID == null || previousSessionKey == null) {
    return false;
  }
  return sessionIDFromDashboardSessionKey(previousSessionKey) !== sessionID;
}

export function shouldResetDashboardSessionScopedState({
  previousSessionKey,
  refreshToken,
  sessionID,
}: {
  previousSessionKey: string | null;
  refreshToken: number;
  sessionID: string | null;
}): boolean {
  if (sessionID == null) {
    return true;
  }
  return previousSessionKey !== null || refreshToken !== 0;
}
