import { parseWorkerArgsText } from "../../current-factory-definition/lib/worker-editable-values";
import {
  buildCanonicalModelOperationsFromDraft,
  validateFactoryGraphAddModelOperationsDraft,
} from "./factory-graph-add-model-operation-draft";
import type {
  FactoryGraphAddEntityDraft,
  FactoryGraphAddEntityFieldErrors,
} from "./factory-graph-editor-additions";
import type { FactoryGraphDraft } from "./factory-graph-draft-types";

type CanonicalWorker = NonNullable<
  FactoryGraphDraft["additions"]["workers"][number]
>;
type ModelProvider = NonNullable<CanonicalWorker["modelProvider"]>;

export function validateFactoryGraphAddWorkerDraft(
  draft: Extract<FactoryGraphAddEntityDraft, { kind: "worker" }>,
): Pick<
  FactoryGraphAddEntityFieldErrors,
  "args" | "command" | "modelOperations" | "modelProvider"
> {
  const errors: Pick<
    FactoryGraphAddEntityFieldErrors,
    "args" | "command" | "modelOperations" | "modelProvider"
  > = {};

  if (
    draft.workerType === "MODEL_WORKER" &&
    draft.modelProvider.trim().length === 0
  ) {
    errors.modelProvider = "Select a model provider for the new worker.";
  }

  if (
    draft.workerType === "SCRIPT_WORKER" &&
    draft.command.trim().length === 0
  ) {
    errors.command = "Enter a command for the new script worker.";
  }

  if (draft.workerType === "SCRIPT_WORKER" && draft.argsText.includes("\0")) {
    errors.args = "Each script argument must be a single non-empty line.";
  }

  if (draft.workerType === "MODEL_WORKER") {
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
  if (entityDraft.workerType === "SCRIPT_WORKER") {
    const args = parseWorkerArgsText(entityDraft.argsText);
    nextDraft.additions.workers.push({
      command: entityDraft.command.trim(),
      ...(args.length > 0 ? { args } : {}),
      name: entityDraft.name.trim(),
      type: "SCRIPT_WORKER",
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
    type: "MODEL_WORKER",
  });
  return nextDraft;
}
