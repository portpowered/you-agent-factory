import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useDashboardInitialReconnectCursor } from "./useDashboardInitialReconnectCursor";

describe("useDashboardInitialReconnectCursor", () => {
  it("returns undefined after same-session refresh even when a checkpoint exists", () => {
    const checkpoint = {
      afterEventId: "factory-event/dispatch-completed/stale-cursor",
      afterSequence: 29,
      selectedTick: 29,
    };

    const { result, rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) =>
        useDashboardInitialReconnectCursor({
          persistedCheckpoint: checkpoint,
          refreshToken,
          sessionID: "~default",
        }),
      { initialProps: { refreshToken: 0 } },
    );

    expect(result.current).toEqual({
      afterEventId: "factory-event/dispatch-completed/stale-cursor",
      afterSequence: 29,
    });

    rerender({ refreshToken: 1 });

    expect(result.current).toBeUndefined();
  });
});
