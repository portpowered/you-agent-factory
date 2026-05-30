import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";

import { FACTORY_EVENT_TYPES } from "../../../api/events";
import {
  currentFactoryDocumentQueryKey,
  currentFactoryDefinitionQueryKey,
} from "../../current-factory-definition/public";
import {
  clearQueuedFlush,
  pausedDashboardStreamState,
  prepareDashboardStreamSession,
  syncCurrentFactoryDefinition,
} from "./dashboard-event-stream";

describe("clearQueuedFlush", () => {
  it("no-ops when there is no pending flush handle", () => {
    const flushHandleRef = { current: null as number | null };
    clearQueuedFlush(flushHandleRef);
    expect(flushHandleRef.current).toBeNull();
  });

  it("cancels animation-frame flushes when supported", () => {
    const cancelAnimationFrame = vi.fn();
    vi.stubGlobal("cancelAnimationFrame", cancelAnimationFrame);
    const flushHandleRef = { current: 42 as number | null };
    clearQueuedFlush(flushHandleRef);
    expect(cancelAnimationFrame).toHaveBeenCalledWith(42);
    expect(flushHandleRef.current).toBeNull();
    vi.unstubAllGlobals();
  });

  it("falls back to clearTimeout when animation frames are unavailable", () => {
    vi.stubGlobal("cancelAnimationFrame", undefined);
    const clearTimeout = vi.fn();
    vi.stubGlobal("clearTimeout", clearTimeout);
    const flushHandleRef = { current: 7 as number | null };
    clearQueuedFlush(flushHandleRef);
    expect(clearTimeout).toHaveBeenCalledWith(7);
    expect(flushHandleRef.current).toBeNull();
    vi.unstubAllGlobals();
  });
});

describe("prepareDashboardStreamSession", () => {
  it("clears queued events and declines to open when session is deselected", () => {
    const queuedEventsRef = { current: [{ id: "event-1" }] as { id: string }[] };
    const hasOpenedStreamRef = { current: true };
    expect(
      prepareDashboardStreamSession({
        hasOpenedStreamRef,
        previousSessionKey: "~default::0",
        queuedEventsRef,
        refreshToken: 0,
        selectedSessionID: null,
      }),
    ).toBe(false);
    expect(queuedEventsRef.current).toEqual([]);
    expect(hasOpenedStreamRef.current).toBe(false);
  });

  it("clears queued events when the session key or refresh token changes", () => {
    const queuedEventsRef = { current: [{ id: "event-1" }] as { id: string }[] };
    const hasOpenedStreamRef = { current: false };
    expect(
      prepareDashboardStreamSession({
        hasOpenedStreamRef,
        previousSessionKey: "~default::0",
        queuedEventsRef,
        refreshToken: 1,
        selectedSessionID: "~default",
      }),
    ).toBe(true);
    expect(queuedEventsRef.current).toEqual([]);
    expect(hasOpenedStreamRef.current).toBe(true);
  });
});

describe("pausedDashboardStreamState", () => {
  it("returns the paused offline message", () => {
    expect(pausedDashboardStreamState()).toEqual({
      status: "offline",
      message: "Live session updates paused. Showing last event state.",
    });
  });
});

describe("syncCurrentFactoryDefinition", () => {
  const sessionID = "~default";
  const validFactory = {
    name: "factory",
    workers: [
      {
        model: "gpt-5.6",
        modelProvider: "CODEX",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
    workstations: [
      {
        body: "Updated prompt",
        id: "review",
        inputs: [{ state: "new", workType: "story" }],
        name: "Review",
        outputs: [],
        promptFile: "prompts/review.md",
        worker: "reviewer",
      },
    ],
  };

  it("ignores non-factory-change events", () => {
    const queryClient = new QueryClient();
    const setQueryData = vi.spyOn(queryClient, "setQueryData");
    syncCurrentFactoryDefinition(queryClient, {
      context: { eventTime: "2026-04-25T20:00:01Z", sequence: 1, tick: 1 },
      id: "event-1",
      payload: {},
      type: FACTORY_EVENT_TYPES.workCreated,
    }, sessionID);
    expect(setQueryData).not.toHaveBeenCalled();
  });

  it("ignores factory-change events without a factory payload", () => {
    const queryClient = new QueryClient();
    const setQueryData = vi.spyOn(queryClient, "setQueryData");
    syncCurrentFactoryDefinition(queryClient, {
      context: { eventTime: "2026-04-25T20:00:01Z", sequence: 1, tick: 1 },
      id: "event-1",
      payload: {},
      type: FACTORY_EVENT_TYPES.factoryChange,
    }, sessionID);
    expect(setQueryData).not.toHaveBeenCalled();
  });

  it("updates definition cache and invalidates the document query when version is absent", async () => {
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    syncCurrentFactoryDefinition(queryClient, {
      context: { eventTime: "2026-04-25T20:00:01Z", sequence: 1, tick: 1 },
      id: "event-1",
      payload: { factory: validFactory },
      type: FACTORY_EVENT_TYPES.factoryChange,
    }, sessionID);
    expect(
      queryClient.getQueryData(currentFactoryDefinitionQueryKey(sessionID)),
    ).toMatchObject({
      workers: [expect.objectContaining({ model: "gpt-5.6" })],
    });
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: currentFactoryDocumentQueryKey(sessionID),
    });
  });

  it("updates both definition and document caches when FACTORY_CHANGE includes version", () => {
    const queryClient = new QueryClient();
    const invalidateQueries = vi.spyOn(queryClient, "invalidateQueries");
    syncCurrentFactoryDefinition(queryClient, {
      context: { eventTime: "2026-04-25T20:00:01Z", sequence: 1, tick: 1 },
      id: "event-1",
      payload: {
        factory: {
          ...validFactory,
          version: {
            logical: "9",
            physical: "2026-05-31T12:00:00Z",
          },
        },
      },
      type: FACTORY_EVENT_TYPES.factoryChange,
    }, sessionID);
    expect(invalidateQueries).not.toHaveBeenCalled();
    expect(
      queryClient.getQueryData(currentFactoryDocumentQueryKey(sessionID)),
    ).toMatchObject({
      version: {
        logical: "9",
        physical: "2026-05-31T12:00:00Z",
      },
      workers: [expect.objectContaining({ model: "gpt-5.6" })],
    });
  });

  it("swallows invalid factory payloads without throwing", () => {
    const queryClient = new QueryClient();
    expect(() => {
      syncCurrentFactoryDefinition(queryClient, {
        context: { eventTime: "2026-04-25T20:00:01Z", sequence: 1, tick: 1 },
        id: "event-1",
        payload: { factory: { name: 123 } },
        type: FACTORY_EVENT_TYPES.factoryChange,
      }, sessionID);
    }).not.toThrow();
  });
});
