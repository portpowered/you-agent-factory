import type { components } from "../generated/openapi";

export type CanonicalFactoryDefinition = components["schemas"]["Factory"];

type FactorySchemas = components["schemas"];
type FactoryRootGuard = FactorySchemas["FactoryGuard"];
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
const FACTORY_KEYS = new Set([
  "factoryDirectory",
  "guards",
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
const FACTORY_GUARD_KEYS = new Set(["model", "modelProvider", "refreshWindow", "type"]);
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
const FACTORY_ROOT_GUARD_TYPE_VALUES = new Set<FactoryRootGuard["type"]>([
  "INFERENCE_THROTTLE_GUARD",
]);
const WORKSTATION_GUARD_TYPE_VALUES = new Set<FactoryGuard["type"]>([
  "VISIT_COUNT",
  "MATCHES_FIELDS",
]);
const INPUT_GUARD_TYPE_VALUES = new Set<FactoryGuard["type"]>([
  "VISIT_COUNT",
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
  return decodeFactoryDefinition(asRecord(factoryPayload), "factory");
}

export function isCanonicalFactoryDefinition(value: unknown): value is CanonicalFactoryDefinition {
  try {
    normalizeFactoryDefinition(value);
    return true;
  } catch {
    return false;
  }
}

function asRecord(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }

  return { ...value };
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
  const guards = readOptionalArray(value, "guards", path, decodeFactoryGuard);
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
  if (guards !== undefined) {
    factory.guards = guards;
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

function decodeFactoryGuard(value: unknown, path: string): FactoryRootGuard {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, FACTORY_GUARD_KEYS, path);

  const guard: FactoryRootGuard = {
    type: readRequiredEnum(record, "type", path, FACTORY_ROOT_GUARD_TYPE_VALUES),
    modelProvider: readRequiredEnum(record, "modelProvider", path, WORKER_MODEL_PROVIDER_VALUES),
    refreshWindow: readRequiredString(record, "refreshWindow", path),
  };
  const model = readOptionalString(record, "model", path);
  if (model !== undefined) {
    guard.model = model;
  }
  return guard;
}

function decodeWorkstationGuard(value: unknown, path: string): FactoryGuard {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, GUARD_KEYS, path);

  const guard: FactoryGuard = {
    type: readRequiredEnum(record, "type", path, WORKSTATION_GUARD_TYPE_VALUES),
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
    type: readRequiredEnum(record, "type", path, INPUT_GUARD_TYPE_VALUES),
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

