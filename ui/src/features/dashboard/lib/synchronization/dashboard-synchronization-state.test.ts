import { describe, expect, it } from "vitest";

import {
  type DashboardSynchronizationShellInput,
  deriveDashboardSynchronizationShellState,
} from "./dashboard-synchronization-state";

const RESTORED_LIVE: DashboardSynchronizationShellInput = {
  checkpoint: { status: "reusable" },
  connectivity: { status: "live" },
  preflight: { status: "validated" },
  replay: { status: "complete" },
  session: "selected",
  stream: { status: "open" },
};

describe("deriveDashboardSynchronizationShellState", () => {
  it.each([
    ["tick zero", RESTORED_LIVE],
    ["a nonzero tick", RESTORED_LIVE],
  ])("treats a quiet reusable checkpoint at %s as current", (_label, input) => {
    expect(deriveDashboardSynchronizationShellState(input)).toEqual({
      error: null,
      isInitialLoading: false,
      status: "current",
    });
  });

  it.each([
    ["preflight", { preflight: { status: "pending" as const } }],
    ["checkpoint hydration", { checkpoint: { status: "pending" as const } }],
    ["replay", { replay: { status: "pending" as const } }],
    ["stream open", { stream: { status: "pending" as const } }],
    ["live connectivity", { connectivity: { status: "connecting" as const } }],
  ])("keeps a cold session loading while %s is pending", (_label, override) => {
    const cold: DashboardSynchronizationShellInput = {
      ...RESTORED_LIVE,
      checkpoint: { status: "absent" },
      ...override,
    };

    expect(deriveDashboardSynchronizationShellState(cold)).toEqual({
      error: null,
      isInitialLoading: true,
      status: "loading",
    });
  });

  it("reports a completed empty replay as known empty", () => {
    expect(
      deriveDashboardSynchronizationShellState({
        ...RESTORED_LIVE,
        checkpoint: { status: "absent" },
      }),
    ).toEqual({
      error: null,
      isInitialLoading: false,
      status: "known_empty",
    });
  });

  it("keeps offline, reconnecting, and recovery failure distinct", () => {
    const offline = deriveDashboardSynchronizationShellState({
      ...RESTORED_LIVE,
      checkpoint: { status: "absent" },
      connectivity: { message: "Stream is offline.", status: "offline" },
    });
    const reconnecting = deriveDashboardSynchronizationShellState({
      ...RESTORED_LIVE,
      connectivity: { message: "Reconnecting.", status: "reconnecting" },
    });
    const recoveryFailed = deriveDashboardSynchronizationShellState({
      ...RESTORED_LIVE,
      connectivity: {
        message: "Automatic recovery failed.",
        status: "recovery_failed",
      },
    });

    expect(offline.status).toBe("offline");
    expect(offline.error?.message).toBe("Stream is offline.");
    expect(reconnecting).toEqual({
      error: null,
      isInitialLoading: false,
      status: "reconnecting",
    });
    expect(recoveryFailed.status).toBe("recovery_failed");
    expect(recoveryFailed.error?.message).toBe("Automatic recovery failed.");
  });

  it("preserves a reusable checkpoint while offline", () => {
    expect(
      deriveDashboardSynchronizationShellState({
        ...RESTORED_LIVE,
        connectivity: { message: "Stream is offline.", status: "offline" },
      }),
    ).toEqual({
      error: null,
      isInitialLoading: false,
      status: "offline",
    });
  });

  it("surfaces preflight failure before synchronization can proceed", () => {
    const error = new Error("Checkpoint validation failed.");
    expect(
      deriveDashboardSynchronizationShellState({
        ...RESTORED_LIVE,
        preflight: { error, status: "failed" },
      }),
    ).toEqual({ error, isInitialLoading: false, status: "failed" });
  });

  it("is idle when no session is selected", () => {
    expect(
      deriveDashboardSynchronizationShellState({
        ...RESTORED_LIVE,
        session: "none",
      }),
    ).toEqual({ error: null, isInitialLoading: false, status: "idle" });
  });
});
