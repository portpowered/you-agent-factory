import { describe, expect, it } from "bun:test";
import {
  listFactorySessions,
  openFactorySession,
} from "../../../api/factory-sessions";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  canonicalizeDashboardRegressionSessions,
  createDashboardRegressionFixture,
  DASHBOARD_REGRESSION_SESSION_IDS,
  dashboardRegressionChartStates,
  dashboardRegressionFactoryOperations,
  dashboardRegressionSessionLists,
  dashboardRegressionSubmitScenarios,
} from ".";

describe("dashboard regression fixture", () => {
  it("keeps selectors, session rows, and chart points keyed by canonical identity", () => {
    const selectorRow = {
      ...dashboardRegressionSessionLists.initial[0],
      id: DEFAULT_FACTORY_SESSION_ID,
    };
    const rows = canonicalizeDashboardRegressionSessions([
      selectorRow,
      dashboardRegressionSessionLists.initial[0],
      dashboardRegressionSessionLists.initial[0],
    ]);

    expect(rows.map((session) => session.id)).toEqual([
      DASHBOARD_REGRESSION_SESSION_IDS.default,
    ]);
    expect(rows.map((session) => session.id)).not.toContain(
      DEFAULT_FACTORY_SESSION_ID,
    );

    const pointIDs = dashboardRegressionChartStates.success.series.completed
      .concat(dashboardRegressionChartStates.success.series.failed)
      .map((point) => point.pointID);
    expect(new Set(pointIDs).size).toBe(pointIDs.length);
    expect(
      dashboardRegressionChartStates.success.series.completed.map(
        (point) => point.sessionID,
      ),
    ).toEqual([
      DASHBOARD_REGRESSION_SESSION_IDS.default,
      DASHBOARD_REGRESSION_SESSION_IDS.secondary,
    ]);
  });

  it("advances list refreshes without allowing an older response to win", async () => {
    const fixture = createDashboardRegressionFixture();
    const stale = fixture.sessionLists.request("stale");
    const refreshed = fixture.sessionLists.request("refreshed");

    fixture.sessionLists.resolve("refreshed");
    fixture.sessionLists.resolve("stale");

    await expect(refreshed).resolves.toEqual(
      dashboardRegressionSessionLists.refreshed,
    );
    await expect(stale).resolves.toEqual(dashboardRegressionSessionLists.stale);
    expect(fixture.state()).toMatchObject({
      currentSessionListID: "refreshed",
      currentSessionIDs: [
        DASHBOARD_REGRESSION_SESSION_IDS.default,
        DASHBOARD_REGRESSION_SESSION_IDS.created,
      ],
      resolvedSelectedSessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
      selectedSessionSelector: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  it("drives chart states and two-session completions with explicit controls", async () => {
    const fixture = createDashboardRegressionFixture();
    for (const stateID of ["loading", "empty", "error", "success"] as const) {
      fixture.setChartState(stateID);
      expect(fixture.state().chartStateID).toBe(stateID);
    }

    const sessionA = fixture.submissions.request("sessionA");
    const sessionB = fixture.submissions.request("sessionB");
    fixture.submissions.resolve("sessionB", "failure");
    fixture.submissions.resolve("sessionA", "success");

    const outcomeB = await sessionB;
    const outcomeA = await sessionA;
    expect(outcomeB).toMatchObject({
      kind: "failure",
      outcomeID: "submit-error-session-b",
    });
    expect(outcomeA).toMatchObject({
      kind: "success",
      outcomeID: "submit-result-session-a",
      response: { sessionId: DASHBOARD_REGRESSION_SESSION_IDS.default },
    });
    expect(fixture.state().completedSubmitOutcomeIDs).toEqual([
      "submit-error-session-b",
      "submit-result-session-a",
    ]);
    expect(
      dashboardRegressionSubmitScenarios.sessionA.draft.sessionID,
    ).not.toBe(dashboardRegressionSubmitScenarios.sessionB.draft.sessionID);
  });

  it("keeps Open/New validation, cancellation, and confirmation outcomes distinct", async () => {
    const fixture = createDashboardRegressionFixture();
    const validation = fixture.factoryJourneys.request(
      "new-validation-success",
    );
    fixture.factoryJourneys.cancel("new");
    fixture.factoryJourneys.resolve("new-validation-success");

    await expect(validation).resolves.toMatchObject({
      kind: "success",
      response: { initsNewFactory: true },
    });
    expect(fixture.state().cancelledJourneys).toEqual(["new"]);
    expect(
      dashboardRegressionFactoryOperations["new-validation-success"].input
        .initNewFactory,
    ).toBeUndefined();

    const failure = fixture.factoryJourneys.request("open-confirm-failure");
    fixture.factoryJourneys.resolve("open-confirm-failure");
    await expect(failure).resolves.toMatchObject({
      kind: "failure",
      error: { code: "INTERNAL_ERROR" },
    });
  });

  it("can drive the typed Factory Sessions API without a live network", async () => {
    const fixture = createDashboardRegressionFixture();
    const list = listFactorySessions({ fetch: fixture.fetch });
    fixture.sessionLists.resolve("initial");
    await expect(list).resolves.toEqual(
      dashboardRegressionSessionLists.initial,
    );

    const operation =
      dashboardRegressionFactoryOperations["open-validation-success"];
    const open = openFactorySession(operation.input, { fetch: fixture.fetch });
    fixture.factoryJourneys.resolve("open-validation-success");
    await expect(open).resolves.toMatchObject({
      targets: [{ ref: { kind: "named", name: "secondary" } }],
    });

    const submit = fixture.fetch(
      `/factory-sessions/${DASHBOARD_REGRESSION_SESSION_IDS.default}/work`,
      {
        body: JSON.stringify(
          dashboardRegressionSubmitScenarios.sessionA.draft.request,
        ),
        method: "POST",
      },
    );
    fixture.submissions.resolve("sessionA");
    const submitResponse = await submit;
    expect(submitResponse.status).toBe(201);
    await expect(submitResponse.json()).resolves.toMatchObject({
      sessionId: DASHBOARD_REGRESSION_SESSION_IDS.default,
    });
  });
});
