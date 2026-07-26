import { describe, expect, it, mock } from "bun:test";
import { act, renderHook } from "@testing-library/react";

import type { TimelineCheckpointStreamIdentity } from "../../../timeline/state/timelineCheckpointPersistence";
import type { FactoryTimelineCheckpoint } from "../../../timeline/state/timeline/storeState";
import {
  TIMELINE_CHECKPOINT_DEBOUNCE_MS,
  type TimelineCheckpointLifecycleDependencies,
  usePersistedTimelineCheckpoint,
} from "./usePersistedTimelineCheckpoint";

const identityA: TimelineCheckpointStreamIdentity = {
  backendScopeID: "backend",
  factorySessionID: "session-a",
  logicalSessionKeyID: "logical-a",
  streamGenerationID: "generation-a",
};

const identityB: TimelineCheckpointStreamIdentity = {
  ...identityA,
  factorySessionID: "session-b",
  logicalSessionKeyID: "logical-b",
  streamGenerationID: "generation-b",
};

function checkpoint(selectedTick: number): FactoryTimelineCheckpoint {
  return {
    materializedWorkOutcomeState: {},
    replayState: {},
    selectedTick,
  } as FactoryTimelineCheckpoint;
}

function createLifecycleDependencies() {
  const callbacks = new Map<number, () => void>();
  let nextHandle = 1;
  const cancel = mock((handle: number) => {
    callbacks.delete(handle);
  });
  const persist = mock(
    async (
      _indexedDB: IDBFactory,
      _checkpoint: FactoryTimelineCheckpoint,
      _identity: TimelineCheckpointStreamIdentity,
    ) => {},
  );
  const schedule = mock((callback: () => void, delay: number) => {
    expect(delay).toBe(TIMELINE_CHECKPOINT_DEBOUNCE_MS);
    const handle = nextHandle;
    nextHandle += 1;
    callbacks.set(handle, callback);
    return handle;
  });
  const dependencies = {
    cancel,
    persist,
    schedule,
  } satisfies TimelineCheckpointLifecycleDependencies;

  return {
    callbacks,
    dependencies,
    flushScheduled() {
      const scheduled = [...callbacks.values()];
      callbacks.clear();
      act(() => {
        for (const callback of scheduled) callback();
      });
    },
  };
}

interface HookProps {
  checkpoint: FactoryTimelineCheckpoint | undefined;
  checkpointsDisabled?: boolean;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
  syncIdentity?: FactoryTimelineCheckpoint["syncIdentity"];
}

function renderCheckpointHook(initialProps: HookProps) {
  const lifecycle = createLifecycleDependencies();
  const hook = renderHook(
    (props: HookProps) =>
      usePersistedTimelineCheckpoint({
        ...props,
        checkpointsDisabled: props.checkpointsDisabled ?? false,
        dependencies: lifecycle.dependencies,
      }),
    { initialProps },
  );
  return { ...hook, ...lifecycle };
}

describe("persisted timeline checkpoint lifecycle", () => {
  it("debounces same-stream updates and persists only the latest checkpoint", () => {
    const first = checkpoint(1);
    const latest = checkpoint(2);
    const { dependencies, flushScheduled, rerender } = renderCheckpointHook({
      checkpoint: first,
      streamIdentity: identityA,
    });

    rerender({ checkpoint: latest, streamIdentity: identityA });

    expect(dependencies.cancel).toHaveBeenCalledTimes(1);
    expect(dependencies.persist).not.toHaveBeenCalled();
    flushScheduled();
    expect(dependencies.persist).toHaveBeenCalledTimes(1);
    expect(dependencies.persist.mock.calls[0]?.slice(1)).toEqual([
      latest,
      identityA,
    ]);
  });

  it("flushes the previous identity before scheduling its replacement", () => {
    const checkpointA = checkpoint(3);
    const checkpointB = checkpoint(4);
    const { dependencies, flushScheduled, rerender } = renderCheckpointHook({
      checkpoint: checkpointA,
      streamIdentity: identityA,
    });

    rerender({ checkpoint: checkpointB, streamIdentity: identityB });

    expect(dependencies.persist.mock.calls[0]?.slice(1)).toEqual([
      checkpointA,
      identityA,
    ]);
    flushScheduled();
    expect(dependencies.persist.mock.calls[1]?.slice(1)).toEqual([
      checkpointB,
      identityB,
    ]);
  });

  it("flushes a pending checkpoint once during unmount", () => {
    const pending = checkpoint(5);
    const { dependencies, flushScheduled, unmount } = renderCheckpointHook({
      checkpoint: pending,
      streamIdentity: identityA,
    });

    unmount();
    flushScheduled();

    expect(dependencies.persist).toHaveBeenCalledTimes(1);
    expect(dependencies.persist.mock.calls[0]?.slice(1)).toEqual([
      pending,
      identityA,
    ]);
  });
});

describe("persisted timeline checkpoint browser lifecycle", () => {
  it("flushes on pagehide without blocking navigation or repeating the write", () => {
    const pending = checkpoint(6);
    const { dependencies, flushScheduled } = renderCheckpointHook({
      checkpoint: pending,
      streamIdentity: identityA,
    });
    const pageHide = new Event("pagehide", { cancelable: true });

    expect(window.dispatchEvent(pageHide)).toBe(true);
    window.dispatchEvent(new Event("pagehide"));
    flushScheduled();

    expect(pageHide.defaultPrevented).toBe(false);
    expect(dependencies.persist).toHaveBeenCalledTimes(1);
  });

  it("flushes on hidden visibility but ignores visible lifecycle events", () => {
    const ownVisibilityState = Object.getOwnPropertyDescriptor(
      document,
      "visibilityState",
    );
    const { dependencies } = renderCheckpointHook({
      checkpoint: checkpoint(7),
      streamIdentity: identityA,
    });

    try {
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "visible",
      });
      document.dispatchEvent(new Event("visibilitychange"));
      expect(dependencies.persist).not.toHaveBeenCalled();

      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        value: "hidden",
      });
      document.dispatchEvent(new Event("visibilitychange"));
      document.dispatchEvent(new Event("visibilitychange"));
      expect(dependencies.persist).toHaveBeenCalledTimes(1);
    } finally {
      if (ownVisibilityState) {
        Object.defineProperty(
          document,
          "visibilityState",
          ownVisibilityState,
        );
      } else {
        Reflect.deleteProperty(document, "visibilityState");
      }
    }
  });

  it("does not schedule disabled or incomplete stream identities", () => {
    const { dependencies, rerender } = renderCheckpointHook({
      checkpoint: checkpoint(8),
      checkpointsDisabled: true,
      streamIdentity: identityA,
    });

    rerender({
      checkpoint: checkpoint(8),
      streamIdentity: { ...identityA, logicalSessionKeyID: "" },
    });

    expect(dependencies.schedule).not.toHaveBeenCalled();
    expect(dependencies.persist).not.toHaveBeenCalled();
  });

  it("attaches the resolved sync identity to the persisted checkpoint", () => {
    const syncIdentity = {
      backendScopeId: "backend",
      factorySessionId: "session-a",
      logicalSessionKeyId: "logical-a",
      streamGenerationId: "generation-a",
    };
    const source = checkpoint(9);
    const { dependencies, flushScheduled } = renderCheckpointHook({
      checkpoint: source,
      streamIdentity: identityA,
      syncIdentity,
    });

    flushScheduled();

    expect(dependencies.persist.mock.calls[0]?.[1]).toEqual({
      ...source,
      syncIdentity,
    });
  });
});
