import type { components } from "../generated/openapi";

export type CanonicalFactoryDefinition = components["schemas"]["Factory"];

type FactorySchemas = components["schemas"];
type FactoryGuard = FactorySchemas["Guard"];
type FactoryInputType = FactorySchemas["InputType"];
type FactoryResource = FactorySchemas["Resource"];
type FactoryResourceRequirement = FactorySchemas["ResourceRequirement"];
type FactoryWorker = FactorySchemas["Worker"];
type FactoryWorkState = FactorySchemas["WorkState"];
type FactoryWorkstation = FactorySchemas["Workstation"];
type FactoryWorkstationCron = FactorySchemas["WorkstationCron"];
type FactoryWorkstationIO = FactorySchemas["WorkstationIO"];
type FactoryWorkstationLimits = FactorySchemas["WorkstationLimits"];
type FactoryWorkType = FactorySchemas["WorkType"];

const GUARD_TYPE_ALIASES: Record<string, string> = {
  VISIT_COUNT: "VISIT_COUNT",
  MATCHES_FIELDS: "MATCHES_FIELDS",
  ALL_CHILDREN_COMPLETE: "ALL_CHILDREN_COMPLETE",
  ANY_CHILD_FAILED: "ANY_CHILD_FAILED",
  SAME_NAME: "SAME_NAME",
  visit_count: "VISIT_COUNT",
  matches_fields: "MATCHES_FIELDS",
  all_children_complete: "ALL_CHILDREN_COMPLETE",
  any_child_failed: "ANY_CHILD_FAILED",
  same_name: "SAME_NAME",
};
const INPUT_KIND_ALIASES: Record<string, FactoryInputType["type"]> = {
  DEFAULT: "DEFAULT",
  default: "DEFAULT",
};
const WORKER_MODEL_PROVIDER_ALIASES: Record<
  string,
  NonNullable<FactoryWorker["modelProvider"]>
> = {
  CLAUDE: "CLAUDE",
  CODEX: "CODEX",
};
const WORKER_PROVIDER_ALIASES: Record<string, NonNullable<FactoryWorker["executorProvider"]>> = {
  SCRIPT_WRAP: "SCRIPT_WRAP",
};
const WORKSTATION_BEHAVIOR_ALIASES: Record<
  string,
  NonNullable<FactoryWorkstation["behavior"]>
> = {
  CRON: "CRON",
  REPEATER: "REPEATER",
  STANDARD: "STANDARD",
  cron: "CRON",
  repeater: "REPEATER",
  standard: "STANDARD",
};
const FACTORY_KEYS = new Set([
  "factoryDirectory",
  "id",
  "inputTypes",
  "metadata",
  "name",
  "resources",
  "sourceDirectory",
  "supportingFiles",
  "workers",
  "workTypes",
  "workstations",
]);
const INPUT_TYPE_KEYS = new Set(["name", "type"]);
const WORK_TYPE_KEYS = new Set(["name", "states"]);
const WORK_STATE_KEYS = new Set(["name", "type"]);
const RESOURCE_KEYS = new Set(["capacity", "name"]);
const WORKER_KEYS = new Set([
  "args",
  "body",
  "command",
  "executorProvider",
  "model",
  "modelProvider",
  "name",
  "resources",
  "skipPermissions",
  "stopToken",
  "timeout",
  "type",
]);
const WORKSTATION_KEYS = new Set([
  "body",
  "copyReferencedScripts",
  "cron",
  "env",
  "guards",
  "id",
  "inputs",
  "behavior",
  "limits",
  "name",
  "onFailure",
  "onRejection",
  "outputSchema",
  "outputs",
  "promptFile",
  "promptTemplate",
  "resources",
  "stopWords",
  "type",
  "worker",
  "workingDirectory",
  "worktree",
]);
const WORKSTATION_IO_KEYS = new Set(["guards", "state", "workType"]);
const GUARD_KEYS = new Set([
  "matchConfig",
  "matchInput",
  "maxVisits",
  "parentInput",
  "spawnedBy",
  "type",
  "workstation",
]);
const WORKSTATION_LIMITS_KEYS = new Set(["maxExecutionTime", "maxRetries"]);
const WORKSTATION_CRON_KEYS = new Set([
  "expiryWindow",
  "jitter",
  "schedule",
  "triggerAtStart",
]);
const RESOURCE_REQUIREMENT_KEYS = new Set(["capacity", "name"]);
const INPUT_KIND_VALUES = new Set<FactoryInputType["type"]>(["DEFAULT"]);
const WORK_STATE_TYPE_VALUES = new Set<FactoryWorkState["type"]>([
  "FAILED",
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
]);
const WORKER_TYPE_VALUES = new Set<NonNullable<FactoryWorker["type"]>>([
  "MODEL_WORKER",
  "SCRIPT_WORKER",
]);
const WORKER_MODEL_PROVIDER_VALUES = new Set<NonNullable<FactoryWorker["modelProvider"]>>([
  "CLAUDE",
  "CODEX",
]);
const WORKER_PROVIDER_VALUES = new Set<NonNullable<FactoryWorker["executorProvider"]>>([
  "SCRIPT_WRAP",
]);
const WORKSTATION_BEHAVIOR_VALUES = new Set<
  NonNullable<FactoryWorkstation["behavior"]>
>([
  "CRON",
  "REPEATER",
  "STANDARD",
]);
const WORKSTATION_TYPE_VALUES = new Set<NonNullable<FactoryWorkstation["type"]>>([
  "LOGICAL_MOVE",
  "MODEL_WORKSTATION",
]);
const GUARD_TYPE_VALUES = new Set<FactoryGuard["type"]>([
  "VISIT_COUNT",
  "MATCHES_FIELDS",
  "ALL_CHILDREN_COMPLETE",
  "ANY_CHILD_FAILED",
  "SAME_NAME",
]);

export class FactoryDefinitionAPIError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = "FactoryDefinitionAPIError";
  }
}

export function normalizeFactoryDefinition(factoryPayload: unknown): CanonicalFactoryDefinition {
  const factory = withAliasedKeys(asRecord(factoryPayload), {
    input_types: "inputTypes",
    source_directory: "sourceDirectory",
    work_types: "workTypes",
  });

  if (Array.isArray(factory.inputTypes)) {
    factory.inputTypes = factory.inputTypes.map((inputType) => {
      const normalizedInputType = withAliasedKeys(asRecord(inputType), {});
      canonicalizeEnumValue(normalizedInputType, "type", INPUT_KIND_ALIASES);
      return normalizedInputType;
    });
  }

  if (Array.isArray(factory.workTypes)) {
    factory.workTypes = factory.workTypes.map((workType) => {
      const normalizedWorkType = withAliasedKeys(asRecord(workType), {});
      if (Array.isArray(normalizedWorkType.states)) {
        normalizedWorkType.states = normalizedWorkType.states.map((state) =>
          withAliasedKeys(asRecord(state), {}),
        );
      }
      return normalizedWorkType;
    });
  }

  if (Array.isArray(factory.resources)) {
    factory.resources = factory.resources.map((resource) =>
      withAliasedKeys(asRecord(resource), {}),
    );
  }

  if (Array.isArray(factory.workers)) {
    factory.workers = factory.workers.map((worker) => {
      const normalizedWorker = withAliasedKeys(mergeDefinitionFields(asRecord(worker)), {
        model_provider: "modelProvider",
        provider: "executorProvider",
        skip_permissions: "skipPermissions",
        stop_token: "stopToken",
      });
      canonicalizeEnumValue(normalizedWorker, "modelProvider", WORKER_MODEL_PROVIDER_ALIASES);
      canonicalizeEnumValue(normalizedWorker, "executorProvider", WORKER_PROVIDER_ALIASES);
      delete normalizedWorker.concurrency;
      delete normalizedWorker.sessionId;
      delete normalizedWorker.session_id;
      normalizedWorker.resources = normalizeResourceRequirements(normalizedWorker.resources);
      return normalizedWorker;
    });
  }

  if (Array.isArray(factory.workstations)) {
    factory.workstations = factory.workstations.map((workstation) =>
      canonicalizeWorkstation(workstation),
    );
  }

  return decodeFactoryDefinition(factory, "factory");
}

export function isCanonicalFactoryDefinition(value: unknown): value is CanonicalFactoryDefinition {
  try {
    normalizeFactoryDefinition(value);
    return true;
  } catch {
    return false;
  }
}

function canonicalizeWorkstation(workstation: unknown): Record<string, unknown> {
  const normalizedWorkstation = withAliasedKeys(mergeDefinitionFields(asRecord(workstation)), {
    copy_referenced_scripts: "copyReferencedScripts",
    on_failure: "onFailure",
    on_rejection: "onRejection",
    output_schema: "outputSchema",
    prompt_file: "promptFile",
    prompt_template: "promptTemplate",
    stop_words: "stopWords",
    working_directory: "workingDirectory",
  });
  normalizeLegacyWorkstationTypeFields(normalizedWorkstation);
  canonicalizeEnumValue(normalizedWorkstation, "behavior", WORKSTATION_BEHAVIOR_ALIASES);
  normalizeWorkstationRuntimeTypeField(normalizedWorkstation);
  normalizeLegacyWorkstationStopAliases(normalizedWorkstation);
  normalizeLegacyWorkstationTimeoutAlias(normalizedWorkstation);
  normalizeLegacyWorkstationResourceAlias(normalizedWorkstation);

  if (Array.isArray(normalizedWorkstation.inputs)) {
    normalizedWorkstation.inputs = normalizedWorkstation.inputs.map((input) =>
      canonicalizeWorkstationIO(input),
    );
  }

  if (Array.isArray(normalizedWorkstation.outputs)) {
    normalizedWorkstation.outputs = normalizedWorkstation.outputs.map((output) =>
      canonicalizeWorkstationIO(output),
    );
  }

  if (normalizedWorkstation.onFailure) {
    normalizedWorkstation.onFailure = canonicalizeWorkstationIO(normalizedWorkstation.onFailure);
  }

  if (normalizedWorkstation.onRejection) {
    normalizedWorkstation.onRejection = canonicalizeWorkstationIO(
      normalizedWorkstation.onRejection,
    );
  }

  normalizedWorkstation.resources = normalizeResourceRequirements(normalizedWorkstation.resources);

  if (Array.isArray(normalizedWorkstation.guards)) {
    normalizedWorkstation.guards = normalizedWorkstation.guards.map((guard) =>
      canonicalizeGuard(withAliasedKeys(asRecord(guard), {
        max_visits: "maxVisits",
      })),
    );
  }

  if (normalizedWorkstation.limits) {
    normalizedWorkstation.limits = withAliasedKeys(asRecord(normalizedWorkstation.limits), {
      max_execution_time: "maxExecutionTime",
      max_retries: "maxRetries",
    });
  }

  if (normalizedWorkstation.cron) {
    normalizedWorkstation.cron = withAliasedKeys(asRecord(normalizedWorkstation.cron), {
      expiry_window: "expiryWindow",
      trigger_at_start: "triggerAtStart",
    });
  }

  return normalizedWorkstation;
}

function canonicalizeWorkstationIO(value: unknown): Record<string, unknown> {
  const normalizedIO = withAliasedKeys(asRecord(value), {
    work_type: "workType",
  });

  if (Array.isArray(normalizedIO.guards)) {
    normalizedIO.guards = normalizedIO.guards.map((guard) =>
      canonicalizeGuard(withAliasedKeys(asRecord(guard), {
        match_input: "matchInput",
        parent_input: "parentInput",
        spawned_by: "spawnedBy",
      })),
    );
  }

  return normalizedIO;
}

function asRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }

  return { ...value };
}

function withAliasedKeys(
  value: Record<string, unknown>,
  aliases: Record<string, string>,
): Record<string, unknown> {
  const normalizedValue = { ...value };

  for (const [legacyKey, canonicalKey] of Object.entries(aliases)) {
    if (normalizedValue[canonicalKey] === undefined && normalizedValue[legacyKey] !== undefined) {
      normalizedValue[canonicalKey] = normalizedValue[legacyKey];
    }
    delete normalizedValue[legacyKey];
  }

  return normalizedValue;
}

function canonicalizeGuard(guard: Record<string, unknown>): Record<string, unknown> {
  const normalizedGuard = { ...guard };
  const guardType = normalizedGuard.type;

  if (typeof guardType === "string" && GUARD_TYPE_ALIASES[guardType] !== undefined) {
    normalizedGuard.type = GUARD_TYPE_ALIASES[guardType];
  }

  return normalizedGuard;
}

function canonicalizeEnumValue<T extends string>(
  value: Record<string, unknown>,
  key: string,
  aliases: Record<string, T>,
): void {
  const rawValue = value[key];
  if (typeof rawValue !== "string") {
    return;
  }

  const canonicalValue = aliases[rawValue];
  if (canonicalValue !== undefined) {
    value[key] = canonicalValue;
  }
}

function mergeDefinitionFields(container: Record<string, unknown>): Record<string, unknown> {
  const normalizedContainer = { ...container };
  const definition = asRecord(normalizedContainer.definition);

  for (const [key, value] of Object.entries(definition)) {
    if (normalizedContainer[key] !== undefined) {
      continue;
    }
    normalizedContainer[key] = value;
  }

  delete normalizedContainer.definition;
  return normalizedContainer;
}

function normalizeLegacyWorkstationTypeFields(workstation: Record<string, unknown>): void {
  if (workstation.behavior !== undefined || typeof workstation.type !== "string") {
    return;
  }

  const workstationBehavior = WORKSTATION_BEHAVIOR_ALIASES[workstation.type];
  if (workstationBehavior === undefined) {
    return;
  }

  workstation.behavior = workstationBehavior;
  delete workstation.type;
}

function normalizeWorkstationRuntimeTypeField(workstation: Record<string, unknown>): void {
  if (workstation.type === undefined && workstation.runtimeType !== undefined) {
    workstation.type = workstation.runtimeType;
  }
  delete workstation.runtimeType;
}

function normalizeLegacyWorkstationStopAliases(workstation: Record<string, unknown>): void {
  mergeLegacyWorkstationStopWords(workstation, "runtimeStopWords");
  mergeLegacyWorkstationStopWords(workstation, "stopToken");
}

function mergeLegacyWorkstationStopWords(
  workstation: Record<string, unknown>,
  legacyKey: string,
): void {
  if (workstation[legacyKey] === undefined) {
    return;
  }

  if (workstation.stopWords === undefined) {
    const stopWords = workstationStopWordsFromBoundaryValue(workstation[legacyKey]);
    if (stopWords.length > 0) {
      workstation.stopWords = stopWords;
    }
  }

  delete workstation[legacyKey];
}

function workstationStopWordsFromBoundaryValue(value: unknown): string[] {
  if (typeof value === "string") {
    const trimmedValue = value.trim();
    return trimmedValue ? [trimmedValue] : [];
  }

  if (!Array.isArray(value)) {
    return [];
  }

  return value.flatMap((item) => {
    if (typeof item !== "string") {
      return [];
    }
    const trimmedValue = item.trim();
    return trimmedValue ? [trimmedValue] : [];
  });
}

function normalizeLegacyWorkstationTimeoutAlias(workstation: Record<string, unknown>): void {
  if (workstation.timeout === undefined) {
    return;
  }

  const limits = withAliasedKeys(asRecord(workstation.limits), {
    max_execution_time: "maxExecutionTime",
    max_retries: "maxRetries",
  });
  if (limits.maxExecutionTime === undefined) {
    limits.maxExecutionTime = workstation.timeout;
  }

  workstation.limits = limits;
  delete workstation.timeout;
}

function normalizeLegacyWorkstationResourceAlias(workstation: Record<string, unknown>): void {
  if (workstation.resources === undefined && workstation.resourceUsage !== undefined) {
    workstation.resources = workstation.resourceUsage;
  }
  delete workstation.resourceUsage;
}

function normalizeResourceRequirements(value: unknown): unknown {
  if (value === undefined || value === null) {
    return value;
  }

  if (Array.isArray(value)) {
    return value.map((item) => normalizeResourceRequirement(item) ?? item);
  }

  return [normalizeResourceRequirement(value) ?? value];
}

function normalizeResourceRequirement(value: unknown): Record<string, unknown> | null {
  if (typeof value === "string") {
    const trimmedValue = value.trim();
    if (!trimmedValue) {
      return null;
    }
    return {
      capacity: 1,
      name: trimmedValue,
    };
  }

  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return null;
  }

  return asRecord(value);
}

function decodeFactoryDefinition(
  value: Record<string, unknown>,
  path: string,
): CanonicalFactoryDefinition {
  rejectUnknownKeys(value, FACTORY_KEYS, path);

  const factory: CanonicalFactoryDefinition = {
    name: readRequiredString(value, "name", path),
  };
  const id = readOptionalString(value, "id", path);
  const factoryDirectory = readOptionalString(value, "factoryDirectory", path);
  const sourceDirectory = readOptionalString(value, "sourceDirectory", path);
  const metadata = readOptionalStringMap(value, "metadata", path);
  const inputTypes = readOptionalArray(value, "inputTypes", path, decodeInputType);
  const workTypes = readOptionalArray(value, "workTypes", path, decodeWorkType);
  const resources = readOptionalArray(value, "resources", path, decodeResource);
  const supportingFiles = readOptionalObject(value, "supportingFiles", path, expectObject);
  const workers = readOptionalArray(value, "workers", path, decodeWorker);
  const workstations = readOptionalArray(value, "workstations", path, decodeWorkstation);

  if (id !== undefined) {
    factory.id = id;
  }
  if (factoryDirectory !== undefined) {
    factory.factoryDirectory = factoryDirectory;
  }
  if (sourceDirectory !== undefined) {
    factory.sourceDirectory = sourceDirectory;
  }
  if (supportingFiles !== undefined) {
    factory.supportingFiles = supportingFiles as CanonicalFactoryDefinition["supportingFiles"];
  }
  if (metadata !== undefined) {
    factory.metadata = metadata;
  }
  if (inputTypes !== undefined) {
    factory.inputTypes = inputTypes;
  }
  if (workTypes !== undefined) {
    factory.workTypes = workTypes;
  }
  if (resources !== undefined) {
    factory.resources = resources;
  }
  if (workers !== undefined) {
    factory.workers = workers;
  }
  if (workstations !== undefined) {
    factory.workstations = workstations;
  }

  return factory;
}

function decodeInputType(value: unknown, path: string): FactoryInputType {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, INPUT_TYPE_KEYS, path);

  return {
    name: readRequiredString(record, "name", path),
    type: readRequiredEnum(record, "type", path, INPUT_KIND_VALUES),
  };
}

function decodeWorkType(value: unknown, path: string): FactoryWorkType {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORK_TYPE_KEYS, path);

  return {
    name: readRequiredString(record, "name", path),
    states: readRequiredArray(record, "states", path, decodeWorkState),
  };
}

function decodeWorkState(value: unknown, path: string): FactoryWorkState {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORK_STATE_KEYS, path);

  return {
    name: readRequiredString(record, "name", path),
    type: readRequiredEnum(record, "type", path, WORK_STATE_TYPE_VALUES),
  };
}

function decodeResource(value: unknown, path: string): FactoryResource {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, RESOURCE_KEYS, path);

  return {
    capacity: readRequiredInteger(record, "capacity", path),
    name: readRequiredString(record, "name", path),
  };
}

function decodeWorker(value: unknown, path: string): FactoryWorker {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKER_KEYS, path);

  const worker: FactoryWorker = {
    name: readRequiredString(record, "name", path),
  };
  const type = readOptionalEnum(record, "type", path, WORKER_TYPE_VALUES);
  const model = readOptionalString(record, "model", path);
  const modelProvider = readOptionalEnum(record, "modelProvider", path, WORKER_MODEL_PROVIDER_VALUES);
  const executorProvider = readOptionalEnum(
    record,
    "executorProvider",
    path,
    WORKER_PROVIDER_VALUES,
  );
  const command = readOptionalString(record, "command", path);
  const args = readOptionalStringArray(record, "args", path);
  const resources = readOptionalArray(record, "resources", path, decodeResourceRequirement);
  const timeout = readOptionalString(record, "timeout", path);
  const stopToken = readOptionalString(record, "stopToken", path);
  const skipPermissions = readOptionalBoolean(record, "skipPermissions", path);
  const body = readOptionalString(record, "body", path);

  if (type !== undefined) {
    worker.type = type;
  }
  if (model !== undefined) {
    worker.model = model;
  }
  if (modelProvider !== undefined) {
    worker.modelProvider = modelProvider;
  }
  if (executorProvider !== undefined) {
    worker.executorProvider = executorProvider;
  }
  if (command !== undefined) {
    worker.command = command;
  }
  if (args !== undefined) {
    worker.args = args;
  }
  if (resources !== undefined) {
    worker.resources = resources;
  }
  if (timeout !== undefined) {
    worker.timeout = timeout;
  }
  if (stopToken !== undefined) {
    worker.stopToken = stopToken;
  }
  if (skipPermissions !== undefined) {
    worker.skipPermissions = skipPermissions;
  }
  if (body !== undefined) {
    worker.body = body;
  }

  return worker;
}

function decodeWorkstation(value: unknown, path: string): FactoryWorkstation {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_KEYS, path);

  const workstation: FactoryWorkstation = {
    inputs: readRequiredArray(record, "inputs", path, decodeWorkstationIO),
    name: readRequiredString(record, "name", path),
    outputs: readRequiredArray(record, "outputs", path, decodeWorkstationIO),
    worker: readRequiredString(record, "worker", path),
  };
  const id = readOptionalString(record, "id", path);
  const behavior = readOptionalEnum(record, "behavior", path, WORKSTATION_BEHAVIOR_VALUES);
  const type = readOptionalEnum(record, "type", path, WORKSTATION_TYPE_VALUES);
  const promptFile = readOptionalString(record, "promptFile", path);
  const outputSchema = readOptionalString(record, "outputSchema", path);
  const limits = readOptionalObject(record, "limits", path, decodeWorkstationLimits);
  const body = readOptionalString(record, "body", path);
  const promptTemplate = readOptionalString(record, "promptTemplate", path);
  const cron = readOptionalObject(record, "cron", path, decodeWorkstationCron);
  const onRejection = readOptionalObject(record, "onRejection", path, decodeWorkstationIO);
  const onFailure = readOptionalObject(record, "onFailure", path, decodeWorkstationIO);
  const resources = readOptionalArray(record, "resources", path, decodeResourceRequirement);
  const copyReferencedScripts = readOptionalBoolean(record, "copyReferencedScripts", path);
  const guards = readOptionalArray(record, "guards", path, decodeWorkstationGuard);
  const stopWords = readOptionalStringArray(record, "stopWords", path);
  const workingDirectory = readOptionalString(record, "workingDirectory", path);
  const worktree = readOptionalString(record, "worktree", path);
  const env = readOptionalStringMap(record, "env", path);

  if (id !== undefined) {
    workstation.id = id;
  }
  if (behavior !== undefined) {
    workstation.behavior = behavior;
  }
  if (type !== undefined) {
    workstation.type = type;
  }
  if (promptFile !== undefined) {
    workstation.promptFile = promptFile;
  }
  if (outputSchema !== undefined) {
    workstation.outputSchema = outputSchema;
  }
  if (limits !== undefined) {
    workstation.limits = limits;
  }
  if (body !== undefined) {
    workstation.body = body;
  }
  if (promptTemplate !== undefined) {
    workstation.promptTemplate = promptTemplate;
  }
  if (cron !== undefined) {
    workstation.cron = cron;
  }
  if (onRejection !== undefined) {
    workstation.onRejection = onRejection;
  }
  if (onFailure !== undefined) {
    workstation.onFailure = onFailure;
  }
  if (resources !== undefined) {
    workstation.resources = resources;
  }
  if (copyReferencedScripts !== undefined) {
    workstation.copyReferencedScripts = copyReferencedScripts;
  }
  if (guards !== undefined) {
    workstation.guards = guards;
  }
  if (stopWords !== undefined) {
    workstation.stopWords = stopWords;
  }
  if (workingDirectory !== undefined) {
    workstation.workingDirectory = workingDirectory;
  }
  if (worktree !== undefined) {
    workstation.worktree = worktree;
  }
  if (env !== undefined) {
    workstation.env = env;
  }

  return workstation;
}

function decodeWorkstationIO(value: unknown, path: string): FactoryWorkstationIO {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_IO_KEYS, path);

  const io: FactoryWorkstationIO = {
    state: readRequiredString(record, "state", path),
    workType: readRequiredString(record, "workType", path),
  };
  const guards = readOptionalArray(record, "guards", path, decodeInputGuard);
  if (guards !== undefined) {
    io.guards = guards;
  }
  return io;
}

function decodeWorkstationGuard(value: unknown, path: string): FactoryGuard {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, GUARD_KEYS, path);

  const guard: FactoryGuard = {
    type: readRequiredEnum(record, "type", path, GUARD_TYPE_VALUES),
  };
  const matchConfig = readOptionalGuardMatchConfig(record, path);
  const workstation = readOptionalString(record, "workstation", path);
  const maxVisits = readOptionalInteger(record, "maxVisits", path);
  if (matchConfig !== undefined) {
    guard.matchConfig = matchConfig;
  }
  if (workstation !== undefined) {
    guard.workstation = workstation;
  }
  if (maxVisits !== undefined) {
    guard.maxVisits = maxVisits;
  }
  return guard;
}

function decodeInputGuard(value: unknown, path: string): FactoryGuard {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, GUARD_KEYS, path);

  const guard: FactoryGuard = {
    type: readRequiredEnum(record, "type", path, GUARD_TYPE_VALUES),
  };
  const matchInput = readOptionalString(record, "matchInput", path);
  const parentInput = readOptionalString(record, "parentInput", path);
  const spawnedBy = readOptionalString(record, "spawnedBy", path);
  if (matchInput !== undefined) {
    guard.matchInput = matchInput;
  }
  if (parentInput !== undefined) {
    guard.parentInput = parentInput;
  }
  if (spawnedBy !== undefined) {
    guard.spawnedBy = spawnedBy;
  }
  return guard;
}

function readOptionalGuardMatchConfig(
  record: Record<string, unknown>,
  path: string,
): FactoryGuard["matchConfig"] | undefined {
  const rawValue = record.matchConfig;
  if (rawValue === undefined) {
    return undefined;
  }
  const matchConfigPath = `${path}.matchConfig`;
  const matchConfig = expectObject(rawValue, matchConfigPath);
  rejectUnknownKeys(matchConfig, new Set(["inputKey"]), matchConfigPath);
  return {
    inputKey: readRequiredString(matchConfig, "inputKey", matchConfigPath),
  };
}

function decodeWorkstationLimits(value: unknown, path: string): FactoryWorkstationLimits {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_LIMITS_KEYS, path);

  const limits: FactoryWorkstationLimits = {};
  const maxRetries = readOptionalInteger(record, "maxRetries", path);
  const maxExecutionTime = readOptionalString(record, "maxExecutionTime", path);
  if (maxRetries !== undefined) {
    limits.maxRetries = maxRetries;
  }
  if (maxExecutionTime !== undefined) {
    limits.maxExecutionTime = maxExecutionTime;
  }
  return limits;
}

function decodeWorkstationCron(value: unknown, path: string): FactoryWorkstationCron {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_CRON_KEYS, path);

  const cron: FactoryWorkstationCron = {
    schedule: readRequiredString(record, "schedule", path),
    triggerAtStart: readOptionalBoolean(record, "triggerAtStart", path) ?? false,
  };
  const jitter = readOptionalString(record, "jitter", path);
  const expiryWindow = readOptionalString(record, "expiryWindow", path);
  if (jitter !== undefined) {
    cron.jitter = jitter;
  }
  if (expiryWindow !== undefined) {
    cron.expiryWindow = expiryWindow;
  }
  return cron;
}

function decodeResourceRequirement(value: unknown, path: string): FactoryResourceRequirement {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, RESOURCE_REQUIREMENT_KEYS, path);

  return {
    capacity: readRequiredInteger(record, "capacity", path),
    name: readRequiredString(record, "name", path),
  };
}

function readOptionalObject<T>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  decode: (input: unknown, valuePath: string) => T,
): T | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  return decode(item, `${path}.${key}`);
}

function readOptionalArray<T>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  decode: (input: unknown, valuePath: string) => T,
): T[] | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (!Array.isArray(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an array.`);
  }
  return item.map((entry, index) => decode(entry, `${path}.${key}[${index}]`));
}

function readRequiredArray<T>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  decode: (input: unknown, valuePath: string) => T,
): T[] {
  if (value[key] === undefined || value[key] === null) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return readOptionalArray(value, key, path, decode) as T[];
}

function readOptionalString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (typeof item !== "string") {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be a string.`);
  }
  return item;
}

function readRequiredString(value: Record<string, unknown>, key: string, path: string): string {
  const item = readOptionalString(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

function readOptionalBoolean(
  value: Record<string, unknown>,
  key: string,
  path: string,
): boolean | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (typeof item !== "boolean") {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be a boolean.`);
  }
  return item;
}

function readOptionalInteger(
  value: Record<string, unknown>,
  key: string,
  path: string,
): number | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (typeof item !== "number" || !Number.isInteger(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an integer.`);
  }
  return item;
}

function readRequiredInteger(value: Record<string, unknown>, key: string, path: string): number {
  const item = readOptionalInteger(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

function readOptionalStringArray(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string[] | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (!Array.isArray(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an array of strings.`);
  }
  return item.map((entry, index) => {
    if (typeof entry !== "string") {
      throw new FactoryDefinitionAPIError(`${path}.${key}[${index}] must be a string.`);
    }
    return entry;
  });
}

function readOptionalStringMap(
  value: Record<string, unknown>,
  key: string,
  path: string,
): Record<string, string> | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }

  const record = expectObject(item, `${path}.${key}`);
  const stringMap: Record<string, string> = {};
  for (const [mapKey, mapValue] of Object.entries(record)) {
    if (typeof mapValue !== "string") {
      throw new FactoryDefinitionAPIError(`${path}.${key}.${mapKey} must be a string.`);
    }
    stringMap[mapKey] = mapValue;
  }
  return stringMap;
}

function readOptionalEnum<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T | undefined {
  const item = readOptionalString(value, key, path);
  if (item === undefined) {
    return undefined;
  }
  if (!allowedValues.has(item as T)) {
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be one of ${Array.from(allowedValues).join(", ")}.`,
    );
  }
  return item as T;
}

function readRequiredEnum<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T {
  const item = readOptionalEnum(value, key, path, allowedValues);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

function expectObject(value: unknown, path: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new FactoryDefinitionAPIError(`${path} must be an object.`);
  }
  return { ...value };
}

function rejectUnknownKeys(
  value: Record<string, unknown>,
  allowedKeys: Set<string>,
  path: string,
): void {
  for (const key of Object.keys(value)) {
    if (allowedKeys.has(key)) {
      continue;
    }
    throw new FactoryDefinitionAPIError(
      `${path}.${key} is not allowed by the generated factory contract.`,
    );
  }
}
