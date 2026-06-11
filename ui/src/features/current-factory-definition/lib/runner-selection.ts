import type { components } from "../../../api/generated/openapi";
import {
  type ApiModelProviderSelection,
  isOpenApiModelProviderSelection,
  normalizeModelProviderSelection,
} from "../../current-selection/workstation-selection/messages/runner-openapi-enums";

export type ModelProviderSelectionSource =
  components["schemas"]["ModelProviderSelectionSource"];

export interface ResolvedModelProviderSelection {
  modelProvider: ApiModelProviderSelection;
  source: ModelProviderSelectionSource;
}

const DEFAULT_MODEL_PROVIDER: ApiModelProviderSelection = "CODEX";

/** Mirrors backend `ResolveModelProviderSelection` precedence for editable factory UI. */
export function resolveModelProviderSelection(
  workstationModelProvider: string | null | undefined,
  factoryModelProvider: string | null | undefined,
  workerModelProvider: string | null | undefined,
): ResolvedModelProviderSelection {
  const workstation = normalizeModelProviderSelection(workstationModelProvider);
  if (workstation && workstation !== "DEFAULT" && isOpenApiModelProviderSelection(workstation)) {
    return { modelProvider: workstation, source: "workstation" };
  }

  const factory = normalizeModelProviderSelection(factoryModelProvider);
  if (factory && factory !== "DEFAULT" && isOpenApiModelProviderSelection(factory)) {
    return { modelProvider: factory, source: "factory" };
  }

  const worker = normalizeModelProviderSelection(workerModelProvider);
  if (worker && isOpenApiModelProviderSelection(worker)) {
    return { modelProvider: worker, source: "worker" };
  }

  return { modelProvider: DEFAULT_MODEL_PROVIDER, source: "operator_default" };
}

/** Backward-compatible alias for existing runner-named imports. */
export type RunnerSelectionSource = ModelProviderSelectionSource;

/** Backward-compatible alias for existing runner-named imports. */
export interface ResolvedRunnerSelection {
  runnerId: ApiModelProviderSelection;
  source: RunnerSelectionSource;
}

/** Backward-compatible alias for existing runner-named imports. */
export function resolveRunnerSelection(
  workstationModelProvider: string | null | undefined,
  factoryModelProvider: string | null | undefined,
  workerModelProvider: string | null | undefined,
): ResolvedRunnerSelection {
  const selection = resolveModelProviderSelection(
    workstationModelProvider,
    factoryModelProvider,
    workerModelProvider,
  );
  return {
    runnerId: selection.modelProvider,
    source: selection.source,
  };
}
