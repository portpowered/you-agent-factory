import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  goDurationFromWorkerTimeoutPicker,
  type WorkerTimeoutUnit,
  workerTimeoutPickerFromGoDuration,
} from "./worker-timeout-duration";
import {
  DEFAULT_WORKER_TYPE,
  EDITABLE_WORKER_TYPES,
  isModelProviderWorkerType,
  isScriptWorkerType,
} from "./worker-workstation-taxonomy";

type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];
type WorkerType = NonNullable<CanonicalWorker["type"]>;

export { EDITABLE_WORKER_TYPES };

type ModelProvider = NonNullable<CanonicalWorker["modelProvider"]>;
type ModelLocality = NonNullable<CanonicalWorker["modelLocality"]>;
type ExecutorProvider = NonNullable<CanonicalWorker["executorProvider"]>;
type HostedProvider = NonNullable<CanonicalWorker["provider"]>;

export const EDITABLE_MODEL_PROVIDERS: ModelProvider[] = [
  "CLAUDE",
  "CODEX",
  "ANTIGRAVITY",
];

export const EDITABLE_MODEL_LOCALITIES: ModelLocality[] = ["LOCAL", "CLOUD"];

export const EDITABLE_EXECUTOR_PROVIDERS: ExecutorProvider[] = ["SCRIPT_WRAP"];

export const EDITABLE_HOSTED_PROVIDERS: HostedProvider[] = ["LINEAR"];

export const EMPTY_HOSTED_LINEAR_EDITABLE_VALUES = {
  authSecretRef: null,
  linearClaimAssigneeField: null,
  linearClaimPresent: false,
  linearMappingState: null,
  linearMappingWorkType: null,
  linearPollInterval: null,
  linearStateIds: [],
  linearTeamIds: [],
} as const;

export const EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS = {
  authSecretRef: "",
  linearClaimAssigneeField: "",
  linearMappingState: "",
  linearMappingWorkType: "",
  linearPollInterval: "",
  linearStateIdsText: "",
  linearTeamIdsText: "",
} as const;

export interface EditableWorkerValues {
  args: string[];
  authSecretRef: string | null;
  body: string | null;
  command: string | null;
  executorProvider: ExecutorProvider | null;
  linearClaimAssigneeField: string | null;
  linearClaimPresent: boolean;
  linearMappingState: string | null;
  linearMappingWorkType: string | null;
  linearPollInterval: string | null;
  linearStateIds: string[];
  linearTeamIds: string[];
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
  authSecretRef: string;
  body: string;
  command: string;
  name: string;
  executorProvider: ExecutorProvider | null;
  linearClaimAssigneeField: string;
  linearMappingState: string;
  linearMappingWorkType: string;
  linearPollInterval: string;
  linearStateIdsText: string;
  linearTeamIdsText: string;
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
    authSecretRef: worker.auth?.secretRef ?? null,
    body: worker.body ?? null,
    command: worker.command ?? null,
    executorProvider: worker.executorProvider ?? null,
    linearClaimAssigneeField: worker.linear?.claim?.assigneeField ?? null,
    linearClaimPresent: worker.linear?.claim != null,
    linearMappingState: worker.linear?.mapping?.state ?? null,
    linearMappingWorkType: worker.linear?.mapping?.workType ?? null,
    linearPollInterval: worker.linear?.pollInterval ?? null,
    linearStateIds: worker.linear?.stateIds ?? [],
    linearTeamIds: worker.linear?.teamIds ?? [],
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
    authSecretRef: values.authSecretRef ?? "",
    body: values.body ?? "",
    command: values.command ?? "",
    name: values.workerName,
    executorProvider: values.executorProvider,
    linearClaimAssigneeField: values.linearClaimAssigneeField ?? "",
    linearMappingState: values.linearMappingState ?? "",
    linearMappingWorkType: values.linearMappingWorkType ?? "",
    linearPollInterval: values.linearPollInterval ?? "",
    linearStateIdsText: formatWorkerArgsText(values.linearStateIds),
    linearTeamIdsText: formatWorkerArgsText(values.linearTeamIds),
    model: values.model ?? "",
    modelLocality: values.modelLocality,
    modelProvider: values.modelProvider,
    provider: values.provider,
    skipPermissions: values.skipPermissions ?? false,
    stopToken: values.stopToken ?? "",
    timeoutAmount: timeoutPicker.amount,
    timeoutUnit: timeoutPicker.unit,
    type: values.type ?? DEFAULT_WORKER_TYPE,
  };
}

export function areEditableWorkerDraftsEqual(
  left: EditableWorkerDraft,
  right: EditableWorkerDraft,
): boolean {
  return (
    left.argsText === right.argsText &&
    left.authSecretRef === right.authSecretRef &&
    left.body === right.body &&
    left.command === right.command &&
    left.executorProvider === right.executorProvider &&
    left.linearClaimAssigneeField === right.linearClaimAssigneeField &&
    left.linearMappingState === right.linearMappingState &&
    left.linearMappingWorkType === right.linearMappingWorkType &&
    left.linearPollInterval === right.linearPollInterval &&
    left.linearStateIdsText === right.linearStateIdsText &&
    left.linearTeamIdsText === right.linearTeamIdsText &&
    left.model === right.model &&
    left.modelLocality === right.modelLocality &&
    left.modelProvider === right.modelProvider &&
    left.name === right.name &&
    left.provider === right.provider &&
    left.skipPermissions === right.skipPermissions &&
    left.stopToken === right.stopToken &&
    left.timeoutAmount === right.timeoutAmount &&
    left.timeoutUnit === right.timeoutUnit &&
    left.type === right.type
  );
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

  if (isModelProviderWorkerType(draft.type)) {
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

  if (isScriptWorkerType(draft.type)) {
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

  return applyHostedWorkerFromDraft(base, draft);
}

function applyHostedWorkerFromDraft(
  base: CanonicalWorker,
  draft: EditableWorkerDraft,
): CanonicalWorker {
  const worker: CanonicalWorker = {
    ...base,
    ...(draft.provider ? { provider: draft.provider } : {}),
  };

  if (draft.provider !== "LINEAR") {
    return worker;
  }

  const trimmedSecretRef = draft.authSecretRef.trim();
  if (trimmedSecretRef.length > 0) {
    worker.auth = { secretRef: trimmedSecretRef };
  }

  const linear = buildHostedLinearConfigFromDraft(draft);
  if (linear) {
    worker.linear = linear;
  }

  return worker;
}

function buildHostedLinearConfigFromDraft(
  draft: EditableWorkerDraft,
): CanonicalWorker["linear"] | undefined {
  const config: NonNullable<CanonicalWorker["linear"]> = {};
  let hasConfig = false;

  const pollInterval = draft.linearPollInterval.trim();
  if (pollInterval.length > 0) {
    config.pollInterval = pollInterval;
    hasConfig = true;
  }

  const teamIds = parseWorkerArgsText(draft.linearTeamIdsText);
  if (teamIds.length > 0) {
    config.teamIds = teamIds;
    hasConfig = true;
  }

  const stateIds = parseWorkerArgsText(draft.linearStateIdsText);
  if (stateIds.length > 0) {
    config.stateIds = stateIds;
    hasConfig = true;
  }

  const workType = draft.linearMappingWorkType.trim();
  const state = draft.linearMappingState.trim();
  if (workType.length > 0 || state.length > 0) {
    config.mapping = {
      ...(workType.length > 0 ? { workType } : {}),
      ...(state.length > 0 ? { state } : {}),
    };
    hasConfig = true;
  }

  const assigneeField = draft.linearClaimAssigneeField.trim();
  if (assigneeField.length > 0) {
    config.claim = { assigneeField };
    hasConfig = true;
  }

  return hasConfig ? config : undefined;
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
