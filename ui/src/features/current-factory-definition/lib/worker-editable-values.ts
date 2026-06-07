import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  goDurationFromWorkerTimeoutPicker,
  type WorkerTimeoutUnit,
  workerTimeoutPickerFromGoDuration,
} from "./worker-timeout-duration";

type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];
type WorkerType = NonNullable<CanonicalWorker["type"]>;
type ModelProvider = NonNullable<CanonicalWorker["modelProvider"]>;
type ModelLocality = NonNullable<CanonicalWorker["modelLocality"]>;
type ExecutorProvider = NonNullable<CanonicalWorker["executorProvider"]>;
type HostedProvider = NonNullable<CanonicalWorker["provider"]>;

export const EDITABLE_WORKER_TYPES: WorkerType[] = [
  "MODEL_WORKER",
  "SCRIPT_WORKER",
  "HOSTED_WORKER",
];

export const EDITABLE_MODEL_PROVIDERS: ModelProvider[] = [
  "CLAUDE",
  "CODEX",
  "CURSOR",
  "GEMINI",
  "KIRO",
  "OPENCODE",
];

export const EDITABLE_MODEL_LOCALITIES: ModelLocality[] = ["LOCAL", "CLOUD"];

export const EDITABLE_EXECUTOR_PROVIDERS: ExecutorProvider[] = ["SCRIPT_WRAP"];

export const EDITABLE_HOSTED_PROVIDERS: HostedProvider[] = ["LINEAR"];

export interface EditableWorkerValues {
  args: string[];
  body: string | null;
  command: string | null;
  executorProvider: ExecutorProvider | null;
  model: string | null;
  modelLocality: ModelLocality | null;
  modelProvider: ModelProvider | null;
  provider: HostedProvider | null;
  skipPermissions: boolean | null;
  stopToken: string | null;
  timeout: string | null;
  type: WorkerType | undefined;
  workerName: string;
  workstationNames: string[];
}

export interface EditableWorkerDraft {
  argsText: string;
  body: string;
  command: string;
  name: string;
  executorProvider: ExecutorProvider | null;
  model: string;
  modelLocality: ModelLocality | null;
  modelProvider: ModelProvider | null;
  provider: HostedProvider | null;
  skipPermissions: boolean;
  stopToken: string;
  timeoutAmount: string;
  timeoutUnit: WorkerTimeoutUnit;
  type: WorkerType;
}

export function resolveEditableWorkerValues(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): EditableWorkerValues | null {
  const workerResolution = resolveCanonicalWorker(factory, workerName);
  if (!workerResolution) {
    return null;
  }

  const { worker } = workerResolution;

  return {
    args: worker.args ?? [],
    body: worker.body ?? null,
    command: worker.command ?? null,
    executorProvider: worker.executorProvider ?? null,
    model: worker.model ?? null,
    modelLocality: worker.modelLocality ?? null,
    modelProvider: worker.modelProvider ?? null,
    provider: worker.provider ?? null,
    skipPermissions: worker.skipPermissions ?? null,
    stopToken: worker.stopToken ?? null,
    timeout: worker.timeout ?? null,
    type: worker.type,
    workerName: worker.name,
    workstationNames: resolveWorkstationNamesReferencingWorker(
      factory,
      workerName,
    ),
  };
}

export function editableWorkerDraftFromValues(
  values: EditableWorkerValues,
): EditableWorkerDraft {
  const timeoutPicker = workerTimeoutPickerFromGoDuration(values.timeout);

  return {
    argsText: formatWorkerArgsText(values.args),
    body: values.body ?? "",
    command: values.command ?? "",
    name: values.workerName,
    executorProvider: values.executorProvider,
    model: values.model ?? "",
    modelLocality: values.modelLocality,
    modelProvider: values.modelProvider,
    provider: values.provider,
    skipPermissions: values.skipPermissions ?? false,
    stopToken: values.stopToken ?? "",
    timeoutAmount: timeoutPicker.amount,
    timeoutUnit: timeoutPicker.unit,
    type: values.type ?? "MODEL_WORKER",
  };
}

export function applyEditableWorkerDraft(
  factory: CanonicalFactoryDefinition,
  workerName: string,
  draft: EditableWorkerDraft,
): CanonicalFactoryDefinition | null {
  const workerResolution = resolveCanonicalWorker(factory, workerName);
  if (!workerResolution || !factory.workers) {
    return null;
  }

  const trimmedName = draft.name.trim();
  const nextWorker = applyWorkerSkipPermissionsFromDraft(
    applyWorkerStopTokenFromDraft(
      applyWorkerTimeoutFromDraft(
        buildWorkerFromDraft(workerResolution.worker, draft),
        draft,
      ),
      draft,
    ),
    draft,
  );

  return {
    ...factory,
    workers: factory.workers.map((worker, index) =>
      index === workerResolution.workerIndex ? nextWorker : worker,
    ),
    workstations: (factory.workstations ?? []).map((workstation) =>
      workstation.worker === workerName
        ? { ...workstation, worker: trimmedName }
        : workstation,
    ),
  };
}

export function parseWorkerArgsText(argsText: string): string[] {
  return argsText
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

export function formatWorkerArgsText(args: string[]): string {
  return args.join("\n");
}

function buildWorkerFromDraft(
  existingWorker: CanonicalWorker,
  draft: EditableWorkerDraft,
): CanonicalWorker {
  const preserved = pickPreservedWorkerFields(existingWorker);
  const base: CanonicalWorker = {
    ...preserved,
    name: draft.name.trim(),
    type: draft.type,
  };

  if (draft.type === "MODEL_WORKER") {
    return {
      ...base,
      ...(draft.modelProvider ? { modelProvider: draft.modelProvider } : {}),
      ...(draft.model.trim().length > 0 ? { model: draft.model.trim() } : {}),
      ...(draft.modelLocality ? { modelLocality: draft.modelLocality } : {}),
      ...(draft.executorProvider
        ? { executorProvider: draft.executorProvider }
        : {}),
    };
  }

  if (draft.type === "SCRIPT_WORKER") {
    const args = parseWorkerArgsText(draft.argsText);
    return {
      ...base,
      ...(draft.command.trim().length > 0
        ? { command: draft.command.trim() }
        : {}),
      ...(args.length > 0 ? { args } : {}),
      ...(draft.body.trim().length > 0 ? { body: draft.body } : {}),
    };
  }

  return {
    ...base,
    ...(draft.provider ? { provider: draft.provider } : {}),
  };
}

function applyWorkerTimeoutFromDraft(
  worker: CanonicalWorker,
  draft: EditableWorkerDraft,
): CanonicalWorker {
  const { timeout: _preservedTimeout, ...withoutTimeout } = worker;
  const timeout = goDurationFromWorkerTimeoutPicker({
    amount: draft.timeoutAmount,
    unit: draft.timeoutUnit,
  });

  return timeout ? { ...withoutTimeout, timeout } : withoutTimeout;
}

function applyWorkerStopTokenFromDraft(
  worker: CanonicalWorker,
  draft: EditableWorkerDraft,
): CanonicalWorker {
  const { stopToken: _preservedStopToken, ...withoutStopToken } = worker;
  const trimmedStopToken = draft.stopToken.trim();

  return trimmedStopToken.length > 0
    ? { ...withoutStopToken, stopToken: trimmedStopToken }
    : withoutStopToken;
}

function applyWorkerSkipPermissionsFromDraft(
  worker: CanonicalWorker,
  draft: EditableWorkerDraft,
): CanonicalWorker {
  const {
    skipPermissions: _preservedSkipPermissions,
    ...withoutSkipPermissions
  } = worker;

  return draft.skipPermissions
    ? { ...withoutSkipPermissions, skipPermissions: true }
    : withoutSkipPermissions;
}

function pickPreservedWorkerFields(
  worker: CanonicalWorker,
): Partial<CanonicalWorker> {
  const preserved: Partial<CanonicalWorker> = {};

  if (worker.resources !== undefined) {
    preserved.resources = worker.resources;
  }
  if (worker.operations !== undefined) {
    preserved.operations = worker.operations;
  }
  if (worker.auth !== undefined) {
    preserved.auth = worker.auth;
  }
  if (worker.linear !== undefined) {
    preserved.linear = worker.linear;
  }

  return preserved;
}

function resolveCanonicalWorker(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): { worker: CanonicalWorker; workerIndex: number } | null {
  const workers = factory.workers ?? [];
  const workerIndex = workers.findIndex((worker) => worker.name === workerName);
  if (workerIndex < 0) {
    return null;
  }

  return {
    worker: workers[workerIndex],
    workerIndex,
  };
}

function resolveWorkstationNamesReferencingWorker(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): string[] {
  return (factory.workstations ?? [])
    .filter((workstation) => workstation.worker === workerName)
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}
