import type { QueryClient } from "@tanstack/react-query";
import type { RefObject } from "react";

import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import type { CurrentFactoryDocument } from "../../../api/current-factory-definition";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import { normalizeFactoryDefinition } from "../../../api/factory-definition";
import {
  currentFactoryDocumentQueryKey,
  currentFactoryDefinitionQueryKey,
} from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";

export function clearQueuedFlush(flushHandleRef: RefObject<number | null>): void {
  if (flushHandleRef.current === null) {
    return;
  }
  if (typeof window.cancelAnimationFrame === "function") {
    window.cancelAnimationFrame(flushHandleRef.current);
  } else {
    window.clearTimeout(flushHandleRef.current);
  }
  flushHandleRef.current = null;
}

export function prepareDashboardStreamSession({
  hasOpenedStreamRef,
  previousSessionKey,
  queuedEventsRef,
  refreshToken,
  selectedSessionID,
}: {
  hasOpenedStreamRef: RefObject<boolean>;
  previousSessionKey: string | null;
  queuedEventsRef: RefObject<FactoryEvent[]>;
  refreshToken: number;
  selectedSessionID: string | null;
}): boolean {
  if (selectedSessionID == null) {
    queuedEventsRef.current = [];
    hasOpenedStreamRef.current = false;
    return false;
  }

  if (previousSessionKey !== null || refreshToken !== 0) {
    queuedEventsRef.current = [];
  }
  hasOpenedStreamRef.current = true;

  return true;
}

export function pausedDashboardStreamState() {
  return {
    status: "offline" as const,
    // hardcoded-ui-copy-exception: non-product-diagnostic
    message: "Live session updates paused. Showing last event state.",
  };
}

export function syncCurrentFactoryDefinition(
  queryClient: QueryClient,
  event: FactoryEvent,
  sessionID: string,
): void {
  if (event.type !== FACTORY_EVENT_TYPES.factoryChange) {
    return;
  }
  const payloadFactory = (event.payload as { factory?: unknown }).factory;
  if (payloadFactory == null) {
    return;
  }
  try {
    const normalizedFactory = normalizeFactoryDefinition(payloadFactory);
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(sessionID),
      normalizedFactory,
    );
    const document = toCurrentFactoryDocumentFromNormalizedFactory(
      normalizedFactory,
    );
    if (document) {
      queryClient.setQueryData(
        currentFactoryDocumentQueryKey(sessionID),
        document,
      );
      return;
    }
    void queryClient.invalidateQueries({
      queryKey: currentFactoryDocumentQueryKey(sessionID),
    });
  } catch {
    return;
  }
}

function toCurrentFactoryDocumentFromNormalizedFactory(
  normalizedFactory: CanonicalFactoryDefinition,
): CurrentFactoryDocument | null {
  const version = normalizedFactory.version;
  if (
    version == null ||
    typeof version !== "object" ||
    (typeof version.logical !== "string" && typeof version.logical !== "number") ||
    typeof version.physical !== "string"
  ) {
    return null;
  }

  return {
    ...normalizedFactory,
    version: {
      logical: String(version.logical),
      physical: version.physical,
    },
  };
}
