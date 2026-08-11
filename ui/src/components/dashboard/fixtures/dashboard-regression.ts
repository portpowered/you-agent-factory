import type {
  FactorySessionSummary,
  FactorySessionTarget,
  OpenFactorySessionResponse,
} from "../../../api/factory-sessions";
import type { components } from "../../../api/generated/openapi";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../api/session-routing";

type SubmitWorkRequest = components["schemas"]["SubmitWorkRequest"];
type SubmitWorkResponse = components["schemas"]["SubmitWorkResponse"];

/** Stable IDs used by every dashboard regression scenario. */
export const DASHBOARD_REGRESSION_SESSION_IDS = {
  default: "019e0000-0000-7000-8000-000000000042",
  secondary: "019e0000-0000-7000-8000-000000000043",
  removed: "019e0000-0000-7000-8000-000000000044",
  created: "019e0000-0000-7000-8000-000000000045",
} as const;

export type DashboardRegressionCanonicalSessionID =
  (typeof DASHBOARD_REGRESSION_SESSION_IDS)[keyof typeof DASHBOARD_REGRESSION_SESSION_IDS];
export type DashboardRegressionSessionSelector =
  | DashboardRegressionCanonicalSessionID
  | typeof DEFAULT_FACTORY_SESSION_ID;

const defaultSession = {
  factoryDir: "/workspace/dashboard-regression/factory",
  folderPath: "/workspace/dashboard-regression",
  id: DASHBOARD_REGRESSION_SESSION_IDS.default,
  isDefault: true,
  project: "dashboard-regression",
  target: { kind: "default" },
} satisfies FactorySessionSummary;

const secondarySession = {
  factoryDir: "/workspace/dashboard-regression/secondary",
  folderPath: "/workspace/dashboard-regression",
  id: DASHBOARD_REGRESSION_SESSION_IDS.secondary,
  isDefault: false,
  project: "secondary",
  target: { kind: "named", name: "secondary" },
} satisfies FactorySessionSummary;

const removedSession = {
  factoryDir: "/workspace/dashboard-regression/removed",
  folderPath: "/workspace/dashboard-regression",
  id: DASHBOARD_REGRESSION_SESSION_IDS.removed,
  isDefault: false,
  project: "removed",
  target: { kind: "named", name: "removed" },
} satisfies FactorySessionSummary;

const createdSession = {
  factoryDir: "/workspace/dashboard-regression/created",
  folderPath: "/workspace/dashboard-regression",
  id: DASHBOARD_REGRESSION_SESSION_IDS.created,
  isDefault: false,
  project: "created",
  target: { kind: "named", name: "created" },
} satisfies FactorySessionSummary;

export const dashboardRegressionSessionByID = Object.freeze({
  [DASHBOARD_REGRESSION_SESSION_IDS.default]: defaultSession,
  [DASHBOARD_REGRESSION_SESSION_IDS.secondary]: secondarySession,
  [DASHBOARD_REGRESSION_SESSION_IDS.removed]: removedSession,
  [DASHBOARD_REGRESSION_SESSION_IDS.created]: createdSession,
}) satisfies Readonly<
  Record<DashboardRegressionCanonicalSessionID, FactorySessionSummary>
>;

export type DashboardRegressionSessionListID =
  | "initial"
  | "stale"
  | "refreshed";

function frozenList<T>(items: readonly T[]): readonly T[] {
  return Object.freeze([...items]);
}

export const dashboardRegressionSessionLists = Object.freeze({
  initial: frozenList([defaultSession, secondarySession]),
  stale: frozenList([defaultSession, secondarySession, removedSession]),
  refreshed: frozenList([defaultSession, createdSession]),
}) satisfies Readonly<
  Record<DashboardRegressionSessionListID, readonly FactorySessionSummary[]>
>;

/** A transport-shaped duplicate response used to prove alias reconciliation. */
export const dashboardRegressionAliasPlusUUIDSessions = frozenList([
  { ...defaultSession, id: DEFAULT_FACTORY_SESSION_ID },
  defaultSession,
  secondarySession,
]);

/** The selector and its resolved UUID are separate identities by design. */
export const dashboardRegressionDefaultDiscovery = Object.freeze({
  requestedSelector: DEFAULT_FACTORY_SESSION_ID,
  resolvedSessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
  resolvedSession: defaultSession,
});

export function resolveDashboardRegressionSessionSelector(
  selector: DashboardRegressionSessionSelector,
): DashboardRegressionCanonicalSessionID | null {
  if (isDefaultFactorySessionID(selector)) {
    return dashboardRegressionDefaultDiscovery.resolvedSessionID;
  }
  const canonicalSessionID = selector as DashboardRegressionCanonicalSessionID;
  return canonicalSessionID in dashboardRegressionSessionByID
    ? canonicalSessionID
    : null;
}

/** Drop selector aliases and duplicate durable identities from expected rows. */
export function canonicalizeDashboardRegressionSessions(
  sessions: readonly FactorySessionSummary[],
): FactorySessionSummary[] {
  const canonicalRows: FactorySessionSummary[] = [];
  const seenSessionIDs = new Set<string>();

  for (const session of sessions) {
    const sessionID = session.id.trim();
    if (!sessionID || isDefaultFactorySessionID(sessionID)) {
      continue;
    }
    if (seenSessionIDs.has(sessionID)) {
      continue;
    }
    seenSessionIDs.add(sessionID);
    canonicalRows.push(session);
  }
  return canonicalRows;
}

export type DashboardRegressionChartStateID =
  | "loading"
  | "empty"
  | "error"
  | "success";

export interface DashboardRegressionChartPoint {
  readonly pointID: string;
  readonly sessionID: DashboardRegressionCanonicalSessionID;
  readonly sequence: number;
  readonly value: number;
}

export type DashboardRegressionChartState =
  | {
      readonly stateID: "loading";
      readonly widgetID: "work-outcomes";
      readonly accessibleDescription: string;
    }
  | {
      readonly stateID: "empty";
      readonly widgetID: "work-outcomes";
      readonly accessibleDescription: string;
      readonly points: readonly DashboardRegressionChartPoint[];
    }
  | {
      readonly stateID: "error";
      readonly widgetID: "work-outcomes";
      readonly accessibleDescription: string;
      readonly errorID: string;
      readonly retryable: true;
    }
  | {
      readonly stateID: "success";
      readonly widgetID: "work-outcomes";
      readonly accessibleDescription: string;
      readonly series: Readonly<
        Record<"completed" | "failed", readonly DashboardRegressionChartPoint[]>
      >;
    };

// hardcoded-ui-copy-exception: deterministic fixture descriptions, not product copy.
export const dashboardRegressionChartStates = Object.freeze({
  loading: {
    stateID: "loading",
    widgetID: "work-outcomes",
    accessibleDescription: "Work outcomes are loading.",
  },
  empty: {
    stateID: "empty",
    widgetID: "work-outcomes",
    accessibleDescription: "No work outcomes are available.",
    points: frozenList([]),
  },
  error: {
    stateID: "error",
    widgetID: "work-outcomes",
    accessibleDescription: "Work outcomes could not be loaded.",
    errorID: "outcomes-refresh-failed",
    retryable: true,
  },
  success: {
    stateID: "success",
    widgetID: "work-outcomes",
    accessibleDescription:
      "Work outcomes: completed 3, 4; failed 1, 2. Values are labeled by outcome.",
    series: {
      completed: frozenList([
        {
          pointID: "outcome-completed-default-001",
          sessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
          sequence: 1,
          value: 3,
        },
        {
          pointID: "outcome-completed-secondary-001",
          sessionID: DASHBOARD_REGRESSION_SESSION_IDS.secondary,
          sequence: 1,
          value: 4,
        },
      ]),
      failed: frozenList([
        {
          pointID: "outcome-failed-default-001",
          sessionID: DASHBOARD_REGRESSION_SESSION_IDS.default,
          sequence: 1,
          value: 1,
        },
        {
          pointID: "outcome-failed-secondary-001",
          sessionID: DASHBOARD_REGRESSION_SESSION_IDS.secondary,
          sequence: 1,
          value: 2,
        },
      ]),
    },
  },
}) satisfies Readonly<
  Record<DashboardRegressionChartStateID, DashboardRegressionChartState>
>;

export const dashboardRegressionChartTransitions = Object.freeze({
  remounts: frozenList([
    { renderID: "work-outcomes-mount-001", reason: "initial" },
    { renderID: "work-outcomes-mount-002", reason: "session-switch" },
  ]),
  resizes: frozenList([
    { resizeID: "work-outcomes-mobile", width: 320 },
    { resizeID: "work-outcomes-desktop", width: 1280 },
  ]),
});

export type DashboardRegressionSubmitScenarioID = "sessionA" | "sessionB";
export interface DashboardRegressionSubmitOutcomeSuccess {
  readonly kind: "success";
  readonly outcomeID: string;
  readonly response: SubmitWorkResponse;
}
export interface DashboardRegressionSubmitOutcomeFailure {
  readonly kind: "failure";
  readonly outcomeID: string;
  readonly error: {
    readonly code: "BAD_REQUEST" | "INTERNAL_ERROR";
    readonly message: string;
  };
}
export type DashboardRegressionSubmitOutcome =
  | DashboardRegressionSubmitOutcomeSuccess
  | DashboardRegressionSubmitOutcomeFailure;
export interface DashboardRegressionSubmitScenario {
  readonly scenarioID: DashboardRegressionSubmitScenarioID;
  readonly sessionID: DashboardRegressionCanonicalSessionID;
  readonly draft: {
    readonly draftID: string;
    readonly sessionID: DashboardRegressionCanonicalSessionID;
    readonly request: SubmitWorkRequest;
  };
  readonly pending: {
    readonly mutationID: string;
    readonly completionControlID: string;
  };
  readonly outcomes: {
    readonly success: DashboardRegressionSubmitOutcomeSuccess;
    readonly failure: DashboardRegressionSubmitOutcomeFailure;
  };
}

function submitScenario(
  scenarioID: DashboardRegressionSubmitScenarioID,
  sessionID: DashboardRegressionCanonicalSessionID,
  suffix: "a" | "b",
): DashboardRegressionSubmitScenario {
  const response: SubmitWorkResponse = {
    accepted: true,
    name: `dashboard-regression-${suffix}`,
    requestId: `request-session-${suffix}`,
    sessionId: sessionID,
    traceId: `trace-session-${suffix}`,
    workId: `work-session-${suffix}`,
    workTypeName: "story",
  };
  return {
    scenarioID,
    sessionID,
    draft: {
      draftID: `draft-session-${suffix}`,
      sessionID,
      request: {
        name: `dashboard-regression-${suffix}`,
        payload: { fixtureID: `submit-session-${suffix}` },
        workTypeName: "story",
      },
    },
    pending: {
      mutationID: `mutation-session-${suffix}`,
      completionControlID: `complete-session-${suffix}`,
    },
    outcomes: {
      success: {
        kind: "success",
        outcomeID: `submit-result-session-${suffix}`,
        response,
      },
      failure: {
        kind: "failure",
        outcomeID: `submit-error-session-${suffix}`,
        error: {
          code: suffix === "a" ? "INTERNAL_ERROR" : "BAD_REQUEST",
          message: `Session ${suffix.toUpperCase()} submission failed.`,
        },
      },
    },
  };
}

export const dashboardRegressionSubmitScenarios = Object.freeze({
  sessionA: submitScenario(
    "sessionA",
    DASHBOARD_REGRESSION_SESSION_IDS.default,
    "a",
  ),
  sessionB: submitScenario(
    "sessionB",
    DASHBOARD_REGRESSION_SESSION_IDS.secondary,
    "b",
  ),
}) satisfies Readonly<
  Record<DashboardRegressionSubmitScenarioID, DashboardRegressionSubmitScenario>
>;

export type DashboardRegressionFactoryOperationID =
  | "open-validation-success"
  | "open-validation-failure"
  | "open-confirm-success"
  | "open-confirm-failure"
  | "new-validation-success"
  | "new-validation-failure"
  | "new-validation-broken-success"
  | "new-confirm-success"
  | "new-confirm-failure";

export interface DashboardRegressionFactoryOperation {
  readonly operationID: DashboardRegressionFactoryOperationID;
  readonly journey: "open" | "new";
  readonly phase: "validation" | "confirmation";
  readonly input: {
    readonly folderPath: string;
    readonly target?: {
      readonly kind: "default" | "named";
      readonly name?: string;
    };
    readonly validateOnly?: boolean;
    readonly initNewFactory?: boolean;
  };
  readonly outcome:
    | {
        readonly kind: "success";
        readonly response: OpenFactorySessionResponse;
      }
    | {
        readonly kind: "failure";
        readonly error: {
          readonly code: "BAD_REQUEST" | "INTERNAL_ERROR";
          readonly message: string;
        };
      };
}

const openTarget = { kind: "named", name: "secondary" } as const;
const openFactoryTarget = {
  factoryDir: secondarySession.factoryDir,
  folderPath: secondarySession.folderPath,
  label: "secondary",
  project: secondarySession.project,
  ref: openTarget,
} satisfies FactorySessionTarget;
const openValidationResponse = {
  targets: [openFactoryTarget],
} satisfies OpenFactorySessionResponse;
const newValidationResponse = {
  folderPath: "/workspace/dashboard-regression-new",
  initsNewFactory: true,
} satisfies OpenFactorySessionResponse;
const brokenNewValidationResponse = {
  folderPath: "/workspace/dashboard-regression-new-broken",
  initsNewFactory: true,
} satisfies OpenFactorySessionResponse;

function factoryOperation(
  operationID: DashboardRegressionFactoryOperationID,
  journey: "open" | "new",
  phase: "validation" | "confirmation",
  folderPath: string,
  target:
    | {
        readonly kind: "default" | "named";
        readonly name?: string;
      }
    | undefined,
  outcome: DashboardRegressionFactoryOperation["outcome"],
  flags: {
    readonly validateOnly?: boolean;
    readonly initNewFactory?: boolean;
  } = {},
): DashboardRegressionFactoryOperation {
  return {
    operationID,
    journey,
    phase,
    input: {
      folderPath,
      ...(target ? { target } : {}),
      ...flags,
    },
    outcome,
  };
}

export const dashboardRegressionFactoryOperations = Object.freeze({
  "open-validation-success": factoryOperation(
    "open-validation-success",
    "open",
    "validation",
    "/workspace/dashboard-regression",
    undefined,
    { kind: "success", response: openValidationResponse },
    { validateOnly: true },
  ),
  "open-validation-failure": factoryOperation(
    "open-validation-failure",
    "open",
    "validation",
    "/workspace/dashboard-regression-missing",
    undefined,
    {
      kind: "failure",
      error: {
        code: "BAD_REQUEST",
        message: "The selected Factory folder could not be read.",
      },
    },
    { validateOnly: true },
  ),
  "open-confirm-success": factoryOperation(
    "open-confirm-success",
    "open",
    "confirmation",
    "/workspace/dashboard-regression",
    openTarget,
    { kind: "success", response: { session: secondarySession } },
  ),
  "open-confirm-failure": factoryOperation(
    "open-confirm-failure",
    "open",
    "confirmation",
    "/workspace/dashboard-regression-missing",
    openTarget,
    {
      kind: "failure",
      error: {
        code: "INTERNAL_ERROR",
        message: "The selected Factory could not be opened.",
      },
    },
  ),
  "new-validation-success": factoryOperation(
    "new-validation-success",
    "new",
    "validation",
    "/workspace/dashboard-regression-new",
    undefined,
    { kind: "success", response: newValidationResponse },
    { validateOnly: true },
  ),
  "new-validation-failure": factoryOperation(
    "new-validation-failure",
    "new",
    "validation",
    "/workspace/dashboard-regression-new-unwritable",
    undefined,
    {
      kind: "failure",
      error: {
        code: "BAD_REQUEST",
        message: "The new Factory target is not writable.",
      },
    },
    { validateOnly: true },
  ),
  "new-validation-broken-success": factoryOperation(
    "new-validation-broken-success",
    "new",
    "validation",
    "/workspace/dashboard-regression-new-broken",
    undefined,
    { kind: "success", response: brokenNewValidationResponse },
    { validateOnly: true },
  ),
  "new-confirm-success": factoryOperation(
    "new-confirm-success",
    "new",
    "confirmation",
    "/workspace/dashboard-regression-new",
    undefined,
    { kind: "success", response: { session: createdSession } },
    { initNewFactory: true },
  ),
  "new-confirm-failure": factoryOperation(
    "new-confirm-failure",
    "new",
    "confirmation",
    "/workspace/dashboard-regression-new-broken",
    undefined,
    {
      kind: "failure",
      error: {
        code: "INTERNAL_ERROR",
        message: "The new Factory could not be created.",
      },
    },
    { initNewFactory: true },
  ),
}) satisfies Readonly<
  Record<
    DashboardRegressionFactoryOperationID,
    DashboardRegressionFactoryOperation
  >
>;

export const dashboardRegressionFactoryJourneyOutcomes = Object.freeze({
  openCancelled: {
    journey: "open",
    outcomeID: "open-cancelled",
    sessionMembershipChanged: false,
  },
  newCancelled: {
    journey: "new",
    outcomeID: "new-cancelled",
    sessionMembershipChanged: false,
  },
});

export const dashboardRegressionFixture = Object.freeze({
  sessions: {
    aliasPlusUUID: dashboardRegressionAliasPlusUUIDSessions,
    byID: dashboardRegressionSessionByID,
    lists: dashboardRegressionSessionLists,
    defaultDiscovery: dashboardRegressionDefaultDiscovery,
  },
  charts: {
    states: dashboardRegressionChartStates,
    transitions: dashboardRegressionChartTransitions,
  },
  submissions: dashboardRegressionSubmitScenarios,
  factoryJourneys: {
    operations: dashboardRegressionFactoryOperations,
    outcomes: dashboardRegressionFactoryJourneyOutcomes,
  },
});
