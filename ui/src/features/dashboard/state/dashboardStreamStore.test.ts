import { beforeEach, describe, expect, it } from "vitest";

import {
  dashboardStreamStateKey,
  getDashboardStreamStateForSession,
  useDashboardStreamStore,
} from "./dashboardStreamStore";

const sessionAIdentity = {
  backendScopeID: "backend-a",
  factorySessionID: "session-a",
  logicalSessionKeyID: "logical-a",
  streamGenerationID: "generation-a",
};

const sessionBIdentity = {
  backendScopeID: "backend-a",
  factorySessionID: "session-b",
  logicalSessionKeyID: "logical-b",
  streamGenerationID: "generation-b",
};

describe("useDashboardStreamStore session projections", () => {
  beforeEach(() => {
    useDashboardStreamStore.getState().resetStreamState();
  });

  it("keeps connection state isolated by canonical session and generation", () => {
    useDashboardStreamStore.getState().setSessionStreamState(
      "session-a",
      sessionAIdentity,
      {
        message: "Session A is live",
        status: "live",
      },
      ["session-a-alias"],
    );
    useDashboardStreamStore
      .getState()
      .setSessionStreamState("session-b", sessionBIdentity, {
        message: "Session B is offline",
        status: "offline",
      });

    const state = useDashboardStreamStore.getState();
    expect(dashboardStreamStateKey("session-a", sessionAIdentity)).toBe(
      "session-a::generation-a",
    );
    expect(
      getDashboardStreamStateForSession(
        "session-a-alias",
        state.sessionStreamStates,
        state.sessionStreamStateKeysBySessionID,
      ),
    ).toMatchObject({ status: "live" });
    expect(
      getDashboardStreamStateForSession(
        "session-b",
        state.sessionStreamStates,
        state.sessionStreamStateKeysBySessionID,
      ),
    ).toMatchObject({ status: "offline" });
    expect(
      state.sessionStreamStates["session-a::generation-a"]?.streamIdentity,
    ).toEqual(sessionAIdentity);
  });

  it("advances one session to a new generation without changing its sibling", () => {
    useDashboardStreamStore
      .getState()
      .setSessionStreamState("session-a", sessionAIdentity, {
        message: "Session A is connecting",
        status: "connecting",
      });
    useDashboardStreamStore
      .getState()
      .setSessionStreamState("session-b", sessionBIdentity, {
        message: "Session B is live",
        status: "live",
      });

    const nextSessionAIdentity = {
      ...sessionAIdentity,
      streamGenerationID: "generation-a-2",
    };
    useDashboardStreamStore
      .getState()
      .setSessionStreamState("session-a", nextSessionAIdentity, {
        message: "Session A is live again",
        status: "live",
      });

    const state = useDashboardStreamStore.getState();
    expect(
      getDashboardStreamStateForSession(
        "session-a",
        state.sessionStreamStates,
        state.sessionStreamStateKeysBySessionID,
      ),
    ).toMatchObject({ status: "live" });
    expect(
      getDashboardStreamStateForSession(
        "session-b",
        state.sessionStreamStates,
        state.sessionStreamStateKeysBySessionID,
      ),
    ).toMatchObject({ status: "live", message: "Session B is live" });
    expect(
      state.sessionStreamStates["session-a::generation-a"]?.streamState.status,
    ).toBe("connecting");
  });
});
