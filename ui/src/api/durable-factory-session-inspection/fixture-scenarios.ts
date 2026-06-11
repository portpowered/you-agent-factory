import {
  builtinInterruptedRecoverableScenario,
  type LoadedInspectionFixtureScenario,
} from "./builtin-scenarios";
import contractFixtures from "./fixtures/durable-session-contract-fixtures.json";
import type {
  FactoryDispatch,
  FactorySessionArtifactDetail,
  FactorySessionArtifactSummary,
  FactorySessionDispatchSummary,
  FactorySessionDurableReadModel,
  FactorySessionDurableSummary,
  FactorySessionResult,
} from "./types";

interface ContractFixtureScenarioRow {
  artifactDetail?: FactorySessionArtifactDetail;
  artifacts?: FactorySessionArtifactSummary[];
  dispatchDetail?: FactoryDispatch;
  dispatches?: FactorySessionDispatchSummary[];
  id: string;
  listSummary?: FactorySessionDurableSummary;
  result?: FactorySessionResult;
  session?: FactorySessionDurableReadModel;
  tags?: {
    orchestrator?: string;
  };
}

function isJavaScriptScenario(row: ContractFixtureScenarioRow): boolean {
  return row.tags?.orchestrator === "JAVASCRIPT";
}

function loadScenarioFromContractRow(
  row: ContractFixtureScenarioRow,
): LoadedInspectionFixtureScenario | null {
  if (!row.session?.sessionId) {
    return null;
  }

  return {
    artifactDetail: row.artifactDetail,
    artifacts: row.artifacts ?? [],
    dispatchDetail: row.dispatchDetail,
    dispatches: row.dispatches ?? [],
    id: row.id,
    listSummary: row.listSummary,
    result: row.result,
    session: row.session,
  };
}

export function loadJavaScriptInspectionFixtureScenarios(): LoadedInspectionFixtureScenario[] {
  const rows = contractFixtures.scenarios as ContractFixtureScenarioRow[];
  const scenarios = rows
    .filter(isJavaScriptScenario)
    .map(loadScenarioFromContractRow)
    .filter(
      (scenario): scenario is LoadedInspectionFixtureScenario =>
        scenario !== null,
    );

  scenarios.push(builtinInterruptedRecoverableScenario());

  return scenarios;
}

export function indexInspectionFixtureScenarios(
  scenarios: LoadedInspectionFixtureScenario[],
): {
  byScenarioId: Map<string, LoadedInspectionFixtureScenario>;
  bySessionId: Map<string, LoadedInspectionFixtureScenario>;
} {
  const byScenarioId = new Map<string, LoadedInspectionFixtureScenario>();
  const bySessionId = new Map<string, LoadedInspectionFixtureScenario>();

  for (const scenario of scenarios) {
    byScenarioId.set(scenario.id, scenario);
    const sessionId = scenario.session?.sessionId;
    if (sessionId) {
      bySessionId.set(sessionId, scenario);
    }
  }

  return { byScenarioId, bySessionId };
}

export function createInspectionFixtureScenarioIndex(
  scenarios: LoadedInspectionFixtureScenario[] = loadJavaScriptInspectionFixtureScenarios(),
) {
  return {
    scenarios,
    ...indexInspectionFixtureScenarios(scenarios),
  };
}
