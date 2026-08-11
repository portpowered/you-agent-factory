import { isDefaultFactorySessionID } from "../session-routing";
import type { FactorySessionSummary } from "./api";

/**
 * Projects a server session listing into the identity-keyed rows consumed by
 * the dashboard. The default selector is a request alias, never a durable
 * tab identity. A canonical default row wins when a compatibility alias and
 * its UUID are both present; an alias-only row is omitted rather than
 * rendered as a phantom session.
 */
export function normalizeFactorySessionList(
  sessions: readonly FactorySessionSummary[],
): FactorySessionSummary[] {
  const normalized: FactorySessionSummary[] = [];
  const seenIDs = new Set<string>();

  for (const session of sessions) {
    const id = session.id.trim();
    if (!id || isDefaultFactorySessionID(id)) {
      continue;
    }
    if (seenIDs.has(id)) {
      continue;
    }
    seenIDs.add(id);
    normalized.push(id === session.id ? session : { ...session, id });
  }

  return normalized;
}
