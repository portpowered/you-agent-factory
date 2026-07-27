import type { FactoryEventReconnectCursor } from "../../../api/events";
import {
  type FactoryTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
} from "../../timeline/public/checkpoint-reconnect";
import { shouldResumeFromPersistedCheckpoint } from "../lib/dashboard-session-key";

export function resolveDashboardInitialReconnectCursor({
  persistedCheckpoint,
  previousSessionKey,
  refreshToken,
  sessionID,
}: {
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  previousSessionKey: string | null;
  refreshToken: number;
  sessionID: string | null;
}): FactoryEventReconnectCursor | undefined {
  return shouldResumeFromPersistedCheckpoint({
    previousSessionKey,
    refreshToken,
    sessionID,
  })
    ? reconnectCursorFromCheckpoint(persistedCheckpoint)
    : undefined;
}
