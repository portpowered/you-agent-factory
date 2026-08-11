import type { FactorySessionSummary } from "../../../api/factory-sessions";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../api/session-routing";
import {
  canonicalizeDashboardRegressionSessions,
  type DashboardRegressionCanonicalSessionID,
  type DashboardRegressionChartStateID,
  type DashboardRegressionFactoryOperation,
  type DashboardRegressionFactoryOperationID,
  type DashboardRegressionSessionListID,
  type DashboardRegressionSessionSelector,
  type DashboardRegressionSubmitOutcome,
  type DashboardRegressionSubmitScenarioID,
  dashboardRegressionDefaultDiscovery,
  dashboardRegressionFactoryOperations,
  dashboardRegressionFixture,
  dashboardRegressionSessionLists,
  dashboardRegressionSubmitScenarios,
  resolveDashboardRegressionSessionSelector,
} from "./dashboard-regression";

type ControlledPromise<T> = {
  readonly promise: Promise<T>;
  readonly reject: (reason?: unknown) => void;
  readonly resolve: (value: T) => void;
};

function createControlledPromise<T>(): ControlledPromise<T> {
  let rejectPromise!: (reason?: unknown) => void;
  let resolvePromise!: (value: T) => void;
  const promise = new Promise<T>((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
  });

  return { promise, reject: rejectPromise, resolve: resolvePromise };
}

export interface DashboardRegressionFixtureState {
  readonly selectedSessionSelector: DashboardRegressionSessionSelector | null;
  readonly resolvedSelectedSessionID: DashboardRegressionCanonicalSessionID | null;
  readonly currentSessionListID: DashboardRegressionSessionListID;
  readonly currentSessionIDs: readonly DashboardRegressionCanonicalSessionID[];
  readonly chartStateID: DashboardRegressionChartStateID;
  readonly pendingSessionListIDs: readonly DashboardRegressionSessionListID[];
  readonly pendingSubmitScenarioIDs: readonly DashboardRegressionSubmitScenarioID[];
  readonly pendingFactoryOperationIDs: readonly DashboardRegressionFactoryOperationID[];
  readonly completedSubmitOutcomeIDs: readonly string[];
  readonly completedFactoryOperationIDs: readonly DashboardRegressionFactoryOperationID[];
  readonly cancelledJourneys: readonly ("open" | "new")[];
}

export type DashboardRegressionFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

export interface DashboardRegressionFixtureController {
  readonly fixture: typeof dashboardRegressionFixture;
  readonly fetch: DashboardRegressionFetch;
  readonly state: () => DashboardRegressionFixtureState;
  readonly selectSession: (
    selector: DashboardRegressionSessionSelector | null,
  ) => void;
  readonly setChartState: (stateID: DashboardRegressionChartStateID) => void;
  readonly sessionLists: {
    readonly enqueueFetch: (
      requestID: DashboardRegressionSessionListID,
    ) => void;
    readonly request: (
      requestID: DashboardRegressionSessionListID,
    ) => Promise<FactorySessionSummary[]>;
    readonly resolve: (requestID: DashboardRegressionSessionListID) => void;
    readonly reject: (
      requestID: DashboardRegressionSessionListID,
      error?: unknown,
    ) => void;
  };
  readonly submissions: {
    readonly request: (
      scenarioID: DashboardRegressionSubmitScenarioID,
    ) => Promise<DashboardRegressionSubmitOutcome>;
    readonly resolve: (
      scenarioID: DashboardRegressionSubmitScenarioID,
      outcome?: "success" | "failure",
    ) => void;
  };
  readonly factoryJourneys: {
    readonly request: (
      operationID: DashboardRegressionFactoryOperationID,
    ) => Promise<DashboardRegressionFactoryOperation["outcome"]>;
    readonly resolve: (
      operationID: DashboardRegressionFactoryOperationID,
    ) => void;
    readonly cancel: (journey: "open" | "new") => void;
  };
}

const sessionListRequestOrder: Readonly<
  Record<DashboardRegressionSessionListID, number>
> = {
  initial: 0,
  stale: 1,
  refreshed: 2,
};

function requestURL(input: RequestInfo | URL): URL {
  if (input instanceof URL) {
    return input;
  }
  if (typeof input === "string") {
    return new URL(input, "http://dashboard-regression.test");
  }
  return new URL(input.url, "http://dashboard-regression.test");
}

function requestMethod(input: RequestInfo | URL, init?: RequestInit): string {
  if (typeof init?.method === "string") {
    return init.method.toUpperCase();
  }
  if (typeof input === "object" && "method" in input) {
    return input.method.toUpperCase();
  }
  return "GET";
}

function requestBody(init?: RequestInit): Record<string, unknown> {
  if (typeof init?.body !== "string") {
    return {};
  }
  return JSON.parse(init.body) as Record<string, unknown>;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
  });
}

function operationMatchesInput(
  operation: DashboardRegressionFactoryOperation,
  input: Record<string, unknown>,
): boolean {
  return (
    operation.input.folderPath === input.folderPath &&
    (operation.input.validateOnly ?? false) === (input.validateOnly ?? false) &&
    (operation.input.initNewFactory ?? false) ===
      (input.initNewFactory ?? false) &&
    JSON.stringify(operation.input.target) === JSON.stringify(input.target)
  );
}

interface FixtureFetchDependencies {
  readonly queuedFetchListIDs: DashboardRegressionSessionListID[];
  readonly requestSessionList: (
    requestID: DashboardRegressionSessionListID,
  ) => Promise<FactorySessionSummary[]>;
  readonly requestSubmit: (
    scenarioID: DashboardRegressionSubmitScenarioID,
  ) => Promise<DashboardRegressionSubmitOutcome>;
  readonly requestFactoryOperation: (
    operationID: DashboardRegressionFactoryOperationID,
  ) => Promise<DashboardRegressionFactoryOperation["outcome"]>;
}

function createFixtureFetch({
  queuedFetchListIDs,
  requestFactoryOperation,
  requestSessionList,
  requestSubmit,
}: FixtureFetchDependencies): DashboardRegressionFetch {
  return async (input, init) => {
    const url = requestURL(input);
    const method = requestMethod(input, init);

    if (method === "GET" && url.pathname === "/factory-sessions") {
      const requestID = queuedFetchListIDs.shift();
      if (!requestID) {
        return jsonResponse(
          {
            code: "INTERNAL_ERROR",
            message: "No fixture list refresh was queued.",
          },
          500,
        );
      }
      return requestSessionList(requestID).then((sessions) =>
        jsonResponse({ sessions }),
      );
    }

    if (method === "POST" && url.pathname === "/factory-sessions") {
      const inputBody = requestBody(init);
      const operation = Object.values(
        dashboardRegressionFactoryOperations,
      ).find((candidate) => operationMatchesInput(candidate, inputBody));
      if (!operation) {
        return jsonResponse(
          {
            code: "BAD_REQUEST",
            message: "Unknown fixture Factory operation.",
          },
          400,
        );
      }
      return requestFactoryOperation(operation.operationID).then((outcome) =>
        outcome.kind === "success"
          ? jsonResponse(outcome.response)
          : jsonResponse(outcome.error, 400),
      );
    }

    if (
      method === "POST" &&
      url.pathname.match(/^\/factory-sessions\/[^/]+\/work$/)
    ) {
      const sessionID = decodeURIComponent(url.pathname.split("/")[2] ?? "");
      const canonicalSessionID = resolveDashboardRegressionSessionSelector(
        sessionID as DashboardRegressionSessionSelector,
      );
      const scenario = Object.values(dashboardRegressionSubmitScenarios).find(
        (candidate) => candidate.sessionID === canonicalSessionID,
      );
      if (!scenario) {
        return jsonResponse(
          {
            code: "BAD_REQUEST",
            message: "Unknown fixture submit session.",
          },
          400,
        );
      }
      return requestSubmit(scenario.scenarioID).then((outcome) =>
        outcome.kind === "success"
          ? jsonResponse(outcome.response, 201)
          : jsonResponse(outcome.error, 400),
      );
    }

    return jsonResponse(
      {
        code: "NOT_FOUND",
        message: "Unknown dashboard regression fixture route.",
      },
      404,
    );
  };
}

export function createDashboardRegressionFixture(): DashboardRegressionFixtureController {
  let selectedSessionSelector: DashboardRegressionSessionSelector | null =
    DEFAULT_FACTORY_SESSION_ID;
  let resolvedSelectedSessionID: DashboardRegressionCanonicalSessionID | null =
    dashboardRegressionDefaultDiscovery.resolvedSessionID;
  let currentSessionListID: DashboardRegressionSessionListID = "initial";
  let currentSessionRows = canonicalizeDashboardRegressionSessions(
    dashboardRegressionSessionLists.initial,
  );
  let lastAppliedListOrder = sessionListRequestOrder.initial;
  let chartStateID: DashboardRegressionChartStateID = "loading";
  const pendingSessionLists = new Map<
    DashboardRegressionSessionListID,
    ControlledPromise<FactorySessionSummary[]>
  >();
  const pendingSubmissions = new Map<
    DashboardRegressionSubmitScenarioID,
    ControlledPromise<DashboardRegressionSubmitOutcome>
  >();
  const pendingFactoryOperations = new Map<
    DashboardRegressionFactoryOperationID,
    ControlledPromise<DashboardRegressionFactoryOperation["outcome"]>
  >();
  const queuedFetchListIDs: DashboardRegressionSessionListID[] = ["initial"];
  const completedSubmitOutcomeIDs: string[] = [];
  const completedFactoryOperationIDs: DashboardRegressionFactoryOperationID[] =
    [];
  const cancelledJourneys: ("open" | "new")[] = [];

  function applySessionList(
    requestID: DashboardRegressionSessionListID,
    rows: FactorySessionSummary[],
  ): void {
    const requestOrder = sessionListRequestOrder[requestID];
    if (requestOrder < lastAppliedListOrder) {
      return;
    }

    lastAppliedListOrder = requestOrder;
    currentSessionListID = requestID;
    currentSessionRows = canonicalizeDashboardRegressionSessions(rows);
    if (
      resolvedSelectedSessionID != null &&
      !currentSessionRows.some(
        (session) => session.id === resolvedSelectedSessionID,
      )
    ) {
      if (isDefaultFactorySessionID(selectedSessionSelector)) {
        const resolvedDefault = currentSessionRows.find(
          (session) => session.isDefault,
        );
        resolvedSelectedSessionID =
          (resolvedDefault?.id as
            | DashboardRegressionCanonicalSessionID
            | undefined) ?? null;
      } else {
        selectedSessionSelector = null;
        resolvedSelectedSessionID = null;
      }
    }
  }

  function requestSessionList(
    requestID: DashboardRegressionSessionListID,
  ): Promise<FactorySessionSummary[]> {
    if (pendingSessionLists.has(requestID)) {
      throw new Error(
        `Dashboard regression list request is already pending: ${requestID}`,
      );
    }
    const controlled = createControlledPromise<FactorySessionSummary[]>();
    pendingSessionLists.set(requestID, controlled);
    return controlled.promise;
  }

  function resolveSessionList(
    requestID: DashboardRegressionSessionListID,
  ): void {
    const controlled = pendingSessionLists.get(requestID);
    if (!controlled) {
      throw new Error(
        `Dashboard regression list request is not pending: ${requestID}`,
      );
    }
    pendingSessionLists.delete(requestID);
    const rows = canonicalizeDashboardRegressionSessions(
      dashboardRegressionSessionLists[requestID],
    );
    applySessionList(requestID, rows);
    controlled.resolve([...rows]);
  }

  function rejectSessionList(
    requestID: DashboardRegressionSessionListID,
    error: unknown = new Error(
      `Dashboard regression list failed: ${requestID}`,
    ),
  ): void {
    const controlled = pendingSessionLists.get(requestID);
    if (!controlled) {
      throw new Error(
        `Dashboard regression list request is not pending: ${requestID}`,
      );
    }
    pendingSessionLists.delete(requestID);
    controlled.reject(error);
  }

  function requestSubmit(
    scenarioID: DashboardRegressionSubmitScenarioID,
  ): Promise<DashboardRegressionSubmitOutcome> {
    if (pendingSubmissions.has(scenarioID)) {
      throw new Error(
        `Dashboard regression submit is already pending: ${scenarioID}`,
      );
    }
    const controlled =
      createControlledPromise<DashboardRegressionSubmitOutcome>();
    pendingSubmissions.set(scenarioID, controlled);
    return controlled.promise;
  }

  function resolveSubmit(
    scenarioID: DashboardRegressionSubmitScenarioID,
    outcomeID: "success" | "failure" = "success",
  ): void {
    const controlled = pendingSubmissions.get(scenarioID);
    if (!controlled) {
      throw new Error(
        `Dashboard regression submit is not pending: ${scenarioID}`,
      );
    }
    pendingSubmissions.delete(scenarioID);
    const outcome =
      dashboardRegressionSubmitScenarios[scenarioID].outcomes[outcomeID];
    completedSubmitOutcomeIDs.push(outcome.outcomeID);
    controlled.resolve(outcome);
  }

  function requestFactoryOperation(
    operationID: DashboardRegressionFactoryOperationID,
  ): Promise<DashboardRegressionFactoryOperation["outcome"]> {
    if (pendingFactoryOperations.has(operationID)) {
      throw new Error(
        `Dashboard regression Factory operation is already pending: ${operationID}`,
      );
    }
    const controlled =
      createControlledPromise<DashboardRegressionFactoryOperation["outcome"]>();
    pendingFactoryOperations.set(operationID, controlled);
    return controlled.promise;
  }

  function resolveFactoryOperation(
    operationID: DashboardRegressionFactoryOperationID,
  ): void {
    const controlled = pendingFactoryOperations.get(operationID);
    if (!controlled) {
      throw new Error(
        `Dashboard regression Factory operation is not pending: ${operationID}`,
      );
    }
    pendingFactoryOperations.delete(operationID);
    completedFactoryOperationIDs.push(operationID);
    controlled.resolve(
      dashboardRegressionFactoryOperations[operationID].outcome,
    );
  }

  const fetch = createFixtureFetch({
    queuedFetchListIDs,
    requestFactoryOperation,
    requestSessionList,
    requestSubmit,
  });

  return {
    fixture: dashboardRegressionFixture,
    fetch,
    state: () => ({
      selectedSessionSelector,
      resolvedSelectedSessionID,
      currentSessionListID,
      currentSessionIDs: currentSessionRows.map(
        (session) => session.id as DashboardRegressionCanonicalSessionID,
      ),
      chartStateID,
      pendingSessionListIDs: [...pendingSessionLists.keys()],
      pendingSubmitScenarioIDs: [...pendingSubmissions.keys()],
      pendingFactoryOperationIDs: [...pendingFactoryOperations.keys()],
      completedSubmitOutcomeIDs: [...completedSubmitOutcomeIDs],
      completedFactoryOperationIDs: [...completedFactoryOperationIDs],
      cancelledJourneys: [...cancelledJourneys],
    }),
    selectSession: (selector) => {
      selectedSessionSelector = selector;
      resolvedSelectedSessionID =
        selector == null
          ? null
          : resolveDashboardRegressionSessionSelector(selector);
    },
    setChartState: (stateID) => {
      chartStateID = stateID;
    },
    sessionLists: {
      enqueueFetch: (requestID) => {
        queuedFetchListIDs.push(requestID);
      },
      request: requestSessionList,
      resolve: resolveSessionList,
      reject: rejectSessionList,
    },
    submissions: {
      request: requestSubmit,
      resolve: resolveSubmit,
    },
    factoryJourneys: {
      request: requestFactoryOperation,
      resolve: resolveFactoryOperation,
      cancel: (journey) => {
        if (!cancelledJourneys.includes(journey)) {
          cancelledJourneys.push(journey);
        }
      },
    },
  };
}
