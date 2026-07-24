import { parseWorkerArgsText } from "../../current-factory-definition/lib/worker-editable-values";
import {
  isModelProviderWorkerType,
  isPollerWorkerType,
  isScriptWorkerType,
} from "../../current-factory-definition/public";
import type { FactoryGraphDraft } from "./draft/factory-graph-draft-types";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
} from "./editor/factory-graph-editor-additions";
import {
  buildCanonicalModelOperationsFromDraft,
  validateFactoryGraphAddModelOperationsDraft,
} from "./factory-graph-add-model-operation-draft";

type CanonicalWorker = NonNullable<
  FactoryGraphDraft["additions"]["workers"][number]
>;
type ModelProvider = NonNullable<CanonicalWorker["modelProvider"]>;

export function validateFactoryGraphAddWorkerDraft(
  draft: Extract<FactoryGraphAddEntityDraft, { kind: "worker" }>,
): Pick<
  FactoryGraphAddEntityFieldErrors,
  "args" | "command" | "modelOperations" | "modelProvider" | "provider"
> {
  const errors: Pick<
    FactoryGraphAddEntityFieldErrors,
    "args" | "command" | "modelOperations" | "modelProvider" | "provider"
  > = {};

  if (
    isModelProviderWorkerType(draft.workerType) &&
    draft.modelProvider.trim().length === 0
  ) {
    errors.modelProvider = "Select a model provider for the new worker.";
  }

  if (
    isScriptWorkerType(draft.workerType) &&
    draft.command.trim().length === 0
  ) {
    errors.command = "Enter a command for the new script worker.";
  }

  if (
    isPollerWorkerType(draft.workerType) &&
    draft.provider.trim().length === 0
  ) {
    errors.provider = "Select a hosted provider for the new poller worker.";
  }

  if (isScriptWorkerType(draft.workerType) && draft.argsText.includes("\0")) {
    errors.args = "Each script argument must be a single non-empty line.";
  }

  if (isModelProviderWorkerType(draft.workerType)) {
    const modelOperationErrors = validateFactoryGraphAddModelOperationsDraft(
      draft.operations,
    );
    if (modelOperationErrors.summary || modelOperationErrors.byIndex) {
      errors.modelOperations = modelOperationErrors;
    }
  }

  return errors;
}

export function applyFactoryGraphAddWorkerDraft(
  nextDraft: FactoryGraphDraft,
  entityDraft: Extract<FactoryGraphAddEntityDraft, { kind: "worker" }>,
): FactoryGraphDraft {
  if (isScriptWorkerType(entityDraft.workerType)) {
    const args = parseWorkerArgsText(entityDraft.argsText);
    nextDraft.additions.workers.push({
      command: entityDraft.command.trim(),
      ...(args.length > 0 ? { args } : {}),
      name: entityDraft.name.trim(),
      type: "SCRIPT_WORKER",
    });
    return nextDraft;
  }

  if (isPollerWorkerType(entityDraft.workerType)) {
    nextDraft.additions.workers.push({
      name: entityDraft.name.trim(),
      provider: entityDraft.provider.trim() as NonNullable<
        CanonicalWorker["provider"]
      >,
      type: "POLLER_WORKER",
    });
    return nextDraft;
  }

  const modelProvider = entityDraft.modelProvider.trim() as ModelProvider;
  const operations = buildCanonicalModelOperationsFromDraft(
    entityDraft.operations,
  );
  nextDraft.additions.workers.push({
    modelProvider,
    ...(entityDraft.model.trim().length > 0
      ? { model: entityDraft.model.trim() }
      : {}),
    ...(operations ? { operations } : {}),
    name: entityDraft.name.trim(),
    type: entityDraft.workerType,
  });
  return nextDraft;
}
