import type { components } from "../../../api/generated/openapi";
import {
  isOpenApiRunnerID,
  normalizeRunnerID,
  type ApiRunnerID,
} from "../../current-selection/workstation-selection/messages/runner-openapi-enums";

export type RunnerSelectionSource =
  components["schemas"]["RunnerSelectionSource"];

export interface ResolvedRunnerSelection {
  runnerId: ApiRunnerID;
  source: RunnerSelectionSource;
}

const DEFAULT_RUNNER_ID: ApiRunnerID = "codex";

/** Mirrors backend `ResolveRunnerSelection` precedence for editable factory UI. */
export function resolveRunnerSelection(
  workstationRunner: string | null | undefined,
  factoryRunner: string | null | undefined,
  workerModelProvider: string | null | undefined,
): ResolvedRunnerSelection {
  const workstation = normalizeRunnerID(workstationRunner);
  if (workstation && isOpenApiRunnerID(workstation)) {
    return { runnerId: workstation, source: "workstation" };
  }

  const factory = normalizeRunnerID(factoryRunner);
  if (factory && isOpenApiRunnerID(factory)) {
    return { runnerId: factory, source: "factory" };
  }

  const legacy = normalizeRunnerID(workerModelProvider);
  if (legacy && isOpenApiRunnerID(legacy)) {
    return { runnerId: legacy, source: "legacy_provider" };
  }

  return { runnerId: DEFAULT_RUNNER_ID, source: "default" };
}
