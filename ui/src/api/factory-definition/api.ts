/**
 * Canonical factory-definition normalization for the dashboard API boundary.
 *
 * This module shapes and validates `Factory` payloads against the generated OpenAPI
 * contract. It performs no HTTP, no `fetch`, and does not import `transport.ts`.
 * Session-scoped factory GET/PUT lives in `ui/src/api/session-factory/`; editor and
 * import adapters delegate there after normalizing documents through this module.
 */
import type { components } from "../generated/openapi";
import { WorkerType, WorkstationType } from "../generated/openapi";
import {
  expectObject,
  FactoryDefinitionAPIError,
  readOptionalArray,
  readOptionalBoolean,
  readOptionalEnum,
  readOptionalEnumArray,
  readOptionalFactoryVersion,
  readOptionalInteger,
  readOptionalNonEmptyString,
  readOptionalNullableString,
  readOptionalObject,
  readOptionalString,
  readOptionalStringArray,
  readOptionalStringMap,
  readRequiredArray,
  readRequiredEnum,
  readRequiredInteger,
  readRequiredNumber,
  readRequiredString,
  readRequiredStringArray,
  rejectUnknownKeys,
} from "./api.decode-helpers";
import {
  decodeModelOperation,
  decodeWorkstationOperationBinding,
} from "./api.model-invoke";

export { FactoryDefinitionAPIError };

export type CanonicalFactoryDefinition = components["schemas"]["Factory"];

type FactorySchemas = components["schemas"];
type FactoryRootGuard = FactorySchemas["FactoryGuard"];
type FactoryWorkstationGuard = FactorySchemas["WorkstationGuard"];
type FactoryInputGuard = FactorySchemas["InputGuard"];
type FactoryHostedLinearWorkerClaim = FactorySchemas["HostedLinearWorkerClaim"];
type FactoryHostedLinearWorkerConfig =
  FactorySchemas["HostedLinearWorkerConfig"];
type FactoryHostedLinearWorkerMapping =
  FactorySchemas["HostedLinearWorkerMapping"];
type FactoryHostedWorkerAuth = FactorySchemas["HostedWorkerAuth"];
type FactoryInvocationExample = FactorySchemas["FactoryInvocationExample"];
type FactoryInvocationOutputContract =
  FactorySchemas["FactoryInvocationOutputContract"];
type FactoryInvocationParameter = FactorySchemas["FactoryInvocationParameter"];
type FactoryInvocationParameterBinding =
  FactorySchemas["FactoryInvocationParameterBinding"];
type FactoryInvocationSignature = FactorySchemas["FactoryInvocationSignature"];
type FactoryNameValue = FactorySchemas["NameValue"];
type FactoryInputType = FactorySchemas["InputType"];
type FactoryResource = FactorySchemas["Resource"];
type FactoryResourceRequirement = FactorySchemas["ResourceRequirement"];
type FactoryRunnerID = FactorySchemas["RunnerID"];
type FactoryLayout = FactorySchemas["FactoryLayout"];
type FactoryLayoutBounds = FactorySchemas["FactoryLayoutBounds"];
type FactoryLayoutEdge = FactorySchemas["FactoryLayoutEdge"];
type FactoryLayoutGroup = FactorySchemas["FactoryLayoutGroup"];
type FactoryLayoutNode = FactorySchemas["FactoryLayoutNode"];
type FactoryLayoutPoint = FactorySchemas["FactoryLayoutPoint"];
type FactoryLayoutPreferences = FactorySchemas["FactoryLayoutPreferences"];
type FactoryLayoutViewport = FactorySchemas["FactoryLayoutViewport"];
type FactoryWorker = FactorySchemas["Worker"];
type FactoryWorkerModelProvider = FactorySchemas["WorkerModelProvider"];
type FactoryWorkState = FactorySchemas["WorkState"];
type FactoryClassificationRoute = FactorySchemas["ClassificationRoute"];
type FactoryWorkstation = FactorySchemas["Workstation"];
type FactoryWorkstationCron = FactorySchemas["WorkstationCron"];
type FactoryWorkstationIO = FactorySchemas["WorkstationIO"];
type FactoryWorkstationLimits = FactorySchemas["WorkstationLimits"];
type FactoryWorkType = FactorySchemas["WorkType"];
type FactoryWorkTypeHandlingBehavior =
  FactorySchemas["WorkTypeHandlingBehavior"];
const FACTORY_KEYS = new Set([
  "description",
  "examples",
  "factoryDirectory",
  "guards",
  "id",
  "inputTypes",
  "invocationSignature",
  "metadata",
  "name",
  "layout",
  "resources",
  "runner",
  "sourceDirectory",
  "supportingFiles",
  "version",
  "workers",
  "workTypes",
  "workstations",
]);
const FACTORY_GUARD_KEYS = new Set([
  "model",
  "modelProvider",
  "refreshWindow",
  "type",
]);
const INVOCATION_SIGNATURE_KEYS = new Set([
  "outputContract",
  "parameters",
  "unknownNamedArgumentPolicy",
]);
const INVOCATION_PARAMETER_KEYS = new Set([
  "aliases",
  "bindings",
  "choices",
  "defaultValue",
  "defaultValues",
  "description",
  "externalName",
  "name",
  "required",
  "sensitive",
  "typeHint",
  "valueMode",
]);
const INVOCATION_PARAMETER_BINDING_KEYS = new Set(["kind", "position"]);
const INVOCATION_OUTPUT_CONTRACT_KEYS = new Set([
  "contentType",
  "description",
  "fileExtension",
  "mode",
  "pathParameter",
]);
const INVOCATION_EXAMPLE_KEYS = new Set(["args", "description", "name"]);
const NAME_VALUE_KEYS = new Set(["id", "locales", "type", "value", "values"]);
const INPUT_TYPE_KEYS = new Set(["name", "type"]);
const WORK_TYPE_KEYS = new Set([
  "description",
  "handlingBehavior",
  "id",
  "name",
  "states",
]);
const WORK_TYPE_HANDLING_BEHAVIOR_VALUES =
  new Set<FactoryWorkTypeHandlingBehavior>(["DEFAULT"]);
const WORK_STATE_KEYS = new Set(["id", "name", "type"]);
const RESOURCE_KEYS = new Set(["capacity", "id", "name"]);
const LAYOUT_KEYS = new Set([
  "edges",
  "groups",
  "nodes",
  "preferences",
  "schemaVersion",
  "viewport",
]);
const LAYOUT_NODE_KEYS = new Set(["id", "locked", "position", "size"]);
const LAYOUT_EDGE_KEYS = new Set(["id", "labelPosition", "waypoints"]);
const LAYOUT_GROUP_KEYS = new Set([
  "bounds",
  "color",
  "id",
  "label",
  "locked",
  "nodeIds",
  "parentGroupId",
]);
const LAYOUT_POINT_KEYS = new Set(["x", "y"]);
const LAYOUT_SIZE_KEYS = new Set(["height", "width"]);
const LAYOUT_BOUNDS_KEYS = new Set(["height", "width", "x", "y"]);
const LAYOUT_VIEWPORT_KEYS = new Set(["x", "y", "zoom"]);
const LAYOUT_PREFERENCES_KEYS = new Set(["direction"]);
const LAYOUT_PREFERENCE_DIRECTION_VALUES = new Set([
  "UP",
  "DOWN",
  "LEFT",
  "RIGHT",
]);
const WORKER_KEYS = new Set([
  "args",
  "auth",
  "body",
  "command",
  "description",
  "executorProvider",
  "linear",
  "model",
  "modelLocality",
  "modelProvider",
  "name",
  "id",
  "provider",
  "resources",
  "operations",
  "skipPermissions",
  "stopToken",
  "timeout",
  "type",
]);
const HOSTED_WORKER_AUTH_KEYS = new Set(["secretRef"]);
const HOSTED_LINEAR_WORKER_KEYS = new Set([
  "claim",
  "mapping",
  "pollInterval",
  "stateIds",
  "teamIds",
]);
const HOSTED_LINEAR_WORKER_MAPPING_KEYS = new Set(["state", "workType"]);
const HOSTED_LINEAR_WORKER_CLAIM_KEYS = new Set(["assigneeField"]);
const WORKSTATION_KEYS = new Set([
  "body",
  "copyReferencedScripts",
  "classificationRoutes",
  "cron",
  "description",
  "env",
  "guards",
  "id",
  "inputs",
  "behavior",
  "limits",
  "name",
  "onContinue",
  "onFailure",
  "onRejection",
  "operation",
  "operationBindings",
  "outputSchema",
  "outputs",
  "promptFile",
  "resources",
  "runner",
  "stopWords",
  "type",
  "worker",
  "workingDirectory",
  "worktree",
]);
const WORKSTATION_IO_KEYS = new Set(["guards", "state", "workType"]);
const CLASSIFICATION_ROUTE_KEYS = new Set(["label", "outputs"]);
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
const INVOCATION_UNKNOWN_NAMED_ARGUMENT_POLICY_VALUES = new Set<
  NonNullable<FactoryInvocationSignature["unknownNamedArgumentPolicy"]>
>(["ALLOW", "COLLECT", "REJECT"]);
const INVOCATION_PARAMETER_TYPE_HINT_VALUES = new Set<
  NonNullable<FactoryInvocationParameter["typeHint"]>
>([
  "BOOLEAN_STRING",
  "DIRECTORY_PATH",
  "FILE_PATH",
  "NUMBER_STRING",
  "PATH",
  "STRING",
]);
const INVOCATION_PARAMETER_VALUE_MODE_VALUES = new Set<
  NonNullable<FactoryInvocationParameter["valueMode"]>
>(["EXACT", "FILE_CONTENTS", "REPEATED", "VARIADIC"]);
const INVOCATION_PARAMETER_BINDING_KIND_VALUES = new Set<
  FactoryInvocationParameterBinding["kind"]
>(["NAMED", "NAMED_REST", "POSITIONAL", "STDIN"]);
const INVOCATION_OUTPUT_CONTRACT_MODE_VALUES = new Set<
  NonNullable<FactoryInvocationOutputContract["mode"]>
>(["FILE", "INLINE", "JSON"]);
const WORK_STATE_TYPE_VALUES = new Set<FactoryWorkState["type"]>([
  "FAILED",
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
]);
const WORKER_TYPE_VALUES = new Set<NonNullable<FactoryWorker["type"]>>([
  WorkerType.INFERENCE_WORKER,
  WorkerType.AGENT_WORKER,
  WorkerType.SCRIPT_WORKER,
  WorkerType.POLLER_WORKER,
  WorkerType.MODEL_WORKER,
  WorkerType.HOSTED_WORKER,
]);
const WORKER_MODEL_PROVIDER_VALUES = new Set<FactoryWorkerModelProvider>([
  "CLAUDE",
  "CODEX",
  "CURSOR",
  "ANTIGRAVITY",
]);
const EXACT_INVOCATION_PLACEHOLDER_PATTERN = /^\$\{([A-Za-z0-9_.-]+)\}$/;
const PROVIDER_IDENTITY_PATTERN =
  /^(?:[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*|ANTIGRAVITY|ANTHROPIC|CLAUDE|CODEX|CURSOR|OPENAI)$/;
const WORKER_PROVIDER_VALUES = new Set<
  NonNullable<FactoryWorker["executorProvider"]>
>(["ACP", "SCRIPT_WRAP"]);
const WORKER_MODEL_LOCALITY_VALUES = new Set<
  NonNullable<FactoryWorker["modelLocality"]>
>(["LOCAL", "CLOUD"]);
const HOSTED_WORKER_PROVIDER_VALUES = new Set<
  NonNullable<FactoryWorker["provider"]>
>(["LINEAR"]);
const RUNNER_ID_VALUES = new Set<FactoryRunnerID>([
  "codex",
  "claude",
  "cursor-cli",
  "antigravity",
]);
const WORKSTATION_BEHAVIOR_VALUES = new Set<
  NonNullable<FactoryWorkstation["behavior"]>
>(["CRON", "POLLER", "REPEATER", "STANDARD"]);
const WORKSTATION_TYPE_VALUES = new Set<
  NonNullable<FactoryWorkstation["type"]>
>([
  WorkstationType.INFERENCE_RUN,
  WorkstationType.AGENT_RUN,
  WorkstationType.SCRIPT_RUN,
  WorkstationType.POLLER_RUN,
  WorkstationType.CLASSIFIER_WORKSTATION,
  WorkstationType.LOGICAL_MOVE,
  WorkstationType.MODEL_INVOKE,
  WorkstationType.MODEL_WORKSTATION,
]);
const FACTORY_ROOT_GUARD_TYPE_VALUES = new Set<FactoryRootGuard["type"]>([
  "INFERENCE_THROTTLE_GUARD",
]);
const WORKSTATION_GUARD_TYPE_VALUES = new Set<FactoryWorkstationGuard["type"]>([
  "VISIT_COUNT",
  "MATCHES_FIELDS",
]);
const INPUT_GUARD_TYPE_VALUES = new Set<FactoryInputGuard["type"]>([
  "VISIT_COUNT",
  "ALL_CHILDREN_COMPLETE",
  "ANY_CHILD_FAILED",
  "SAME_NAME",
  "SAME_TRACE_ID",
]);

export function normalizeFactoryDefinition(
  factoryPayload: unknown,
): CanonicalFactoryDefinition {
  return decodeFactoryDefinition(asRecord(factoryPayload), "factory");
}

export function isCanonicalFactoryDefinition(
  value: unknown,
): value is CanonicalFactoryDefinition {
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
  const description = readOptionalObject(
    value,
    "description",
    path,
    decodeNameValue,
  );
  const examples = readOptionalArray(
    value,
    "examples",
    path,
    decodeInvocationExample,
  );
  const factoryDirectory = readOptionalString(value, "factoryDirectory", path);
  const sourceDirectory = readOptionalString(value, "sourceDirectory", path);
  const metadata = readOptionalStringMap(value, "metadata", path);
  const inputTypes = readOptionalArray(
    value,
    "inputTypes",
    path,
    decodeInputType,
  );
  const guards = readOptionalArray(value, "guards", path, decodeFactoryGuard);
  const workTypes = readOptionalArray(value, "workTypes", path, decodeWorkType);
  const resources = readOptionalArray(value, "resources", path, decodeResource);
  const layout = readOptionalObject(value, "layout", path, decodeFactoryLayout);
  const invocationSignature = readOptionalObject(
    value,
    "invocationSignature",
    path,
    decodeInvocationSignature,
  );
  const runner = readOptionalEnum(value, "runner", path, RUNNER_ID_VALUES);
  const supportingFiles = readOptionalObject(
    value,
    "supportingFiles",
    path,
    expectObject,
  );
  const version = readOptionalFactoryVersion(value, "version", path);
  const workers = readOptionalArray(
    value,
    "workers",
    path,
    (entry, entryPath) => decodeWorker(entry, entryPath, invocationSignature),
  );
  const workstations = readOptionalArray(
    value,
    "workstations",
    path,
    decodeWorkstation,
  );

  if (id !== undefined) {
    factory.id = id;
  }
  if (description !== undefined) {
    factory.description = description;
  }
  if (examples !== undefined) {
    factory.examples = examples;
  }
  if (factoryDirectory !== undefined) {
    factory.factoryDirectory = factoryDirectory;
  }
  if (sourceDirectory !== undefined) {
    factory.sourceDirectory = sourceDirectory;
  }
  if (supportingFiles !== undefined) {
    factory.supportingFiles =
      supportingFiles as CanonicalFactoryDefinition["supportingFiles"];
  }
  if (version !== undefined) {
    factory.version = version;
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
  if (layout !== undefined) {
    factory.layout = layout;
  }
  if (invocationSignature !== undefined) {
    factory.invocationSignature = invocationSignature;
  }
  if (runner !== undefined) {
    factory.runner = runner;
  }
  if (workers !== undefined) {
    factory.workers = workers;
  }
  if (workstations !== undefined) {
    factory.workstations = workstations;
  }

  return factory;
}

function decodeInvocationSignature(
  value: unknown,
  path: string,
): FactoryInvocationSignature {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, INVOCATION_SIGNATURE_KEYS, path);

  const signature: FactoryInvocationSignature = {};
  const parameters = readOptionalArray(
    record,
    "parameters",
    path,
    decodeInvocationParameter,
  );
  const unknownNamedArgumentPolicy = readOptionalEnum(
    record,
    "unknownNamedArgumentPolicy",
    path,
    INVOCATION_UNKNOWN_NAMED_ARGUMENT_POLICY_VALUES,
  );
  const outputContract = readOptionalObject(
    record,
    "outputContract",
    path,
    decodeInvocationOutputContract,
  );

  if (parameters !== undefined) {
    signature.parameters = parameters;
  }
  if (unknownNamedArgumentPolicy !== undefined) {
    signature.unknownNamedArgumentPolicy = unknownNamedArgumentPolicy;
  }
  if (outputContract !== undefined) {
    signature.outputContract = outputContract;
  }
  return signature;
}

function decodeInvocationParameter(
  value: unknown,
  path: string,
): FactoryInvocationParameter {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, INVOCATION_PARAMETER_KEYS, path);

  const parameter: FactoryInvocationParameter = {
    name: readRequiredString(record, "name", path),
  };
  const description = readOptionalString(record, "description", path);
  const externalName = readOptionalString(record, "externalName", path);
  const aliases = readOptionalStringArray(record, "aliases", path);
  const typeHint = readOptionalEnum(
    record,
    "typeHint",
    path,
    INVOCATION_PARAMETER_TYPE_HINT_VALUES,
  );
  const valueMode = readOptionalEnum(
    record,
    "valueMode",
    path,
    INVOCATION_PARAMETER_VALUE_MODE_VALUES,
  );
  const required = readOptionalBoolean(record, "required", path);
  const sensitive = readOptionalBoolean(record, "sensitive", path);
  const choices = readOptionalStringArray(record, "choices", path);
  const defaultValue = readOptionalString(record, "defaultValue", path);
  const defaultValues = readOptionalStringArray(record, "defaultValues", path);
  const bindings = readOptionalArray(
    record,
    "bindings",
    path,
    decodeInvocationParameterBinding,
  );

  if (description !== undefined) {
    parameter.description = description;
  }
  if (externalName !== undefined) {
    parameter.externalName = externalName;
  }
  if (aliases !== undefined) {
    parameter.aliases = aliases;
  }
  if (typeHint !== undefined) {
    parameter.typeHint = typeHint;
  }
  if (valueMode !== undefined) {
    parameter.valueMode = valueMode;
  }
  if (required !== undefined) {
    parameter.required = required;
  }
  if (sensitive !== undefined) {
    parameter.sensitive = sensitive;
  }
  if (choices !== undefined) {
    parameter.choices = choices;
  }
  if (defaultValue !== undefined) {
    parameter.defaultValue = defaultValue;
  }
  if (defaultValues !== undefined) {
    parameter.defaultValues = defaultValues;
  }
  if (bindings !== undefined) {
    parameter.bindings = bindings;
  }
  return parameter;
}

function decodeInvocationParameterBinding(
  value: unknown,
  path: string,
): FactoryInvocationParameterBinding {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, INVOCATION_PARAMETER_BINDING_KEYS, path);

  const binding: FactoryInvocationParameterBinding = {
    kind: readRequiredEnum(
      record,
      "kind",
      path,
      INVOCATION_PARAMETER_BINDING_KIND_VALUES,
    ),
  };
  const position = readOptionalInteger(record, "position", path);
  if (position !== undefined) {
    binding.position = position;
  }
  return binding;
}

function decodeInvocationOutputContract(
  value: unknown,
  path: string,
): FactoryInvocationOutputContract {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, INVOCATION_OUTPUT_CONTRACT_KEYS, path);

  const outputContract: FactoryInvocationOutputContract = {};
  const mode = readOptionalEnum(
    record,
    "mode",
    path,
    INVOCATION_OUTPUT_CONTRACT_MODE_VALUES,
  );
  const pathParameter = readOptionalString(record, "pathParameter", path);
  const contentType = readOptionalString(record, "contentType", path);
  const fileExtension = readOptionalString(record, "fileExtension", path);
  const description = readOptionalString(record, "description", path);

  if (mode !== undefined) {
    outputContract.mode = mode;
  }
  if (pathParameter !== undefined) {
    outputContract.pathParameter = pathParameter;
  }
  if (contentType !== undefined) {
    outputContract.contentType = contentType;
  }
  if (fileExtension !== undefined) {
    outputContract.fileExtension = fileExtension;
  }
  if (description !== undefined) {
    outputContract.description = description;
  }
  return outputContract;
}

function decodeInvocationExample(
  value: unknown,
  path: string,
): FactoryInvocationExample {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, INVOCATION_EXAMPLE_KEYS, path);

  const example: FactoryInvocationExample = {
    args: decodeInvocationExampleArgs(record.args, `${path}.args`),
    description: decodeNameValue(record.description, `${path}.description`),
    name: readRequiredString(record, "name", path),
  };
  return example;
}

function decodeNameValue(value: unknown, path: string): FactoryNameValue {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, NAME_VALUE_KEYS, path);
  const result: FactoryNameValue = {
    type: readRequiredEnum(
      record,
      "type",
      path,
      new Set(["LOCALIZABLE_ASSET"]),
    ),
    value: readRequiredString(record, "value", path),
  };
  const id = readOptionalString(record, "id", path);
  const locales = readOptionalStringArray(record, "locales", path);
  const values = readOptionalStringMap(record, "values", path);
  if (id !== undefined) result.id = id;
  if (locales !== undefined) result.locales = locales;
  if (values !== undefined) result.values = values;
  return result;
}

function decodeInvocationExampleArgs(value: unknown, path: string) {
  const record = expectObject(value, path);
  const args: Record<string, string | string[]> = {};
  for (const [key, item] of Object.entries(record)) {
    if (typeof item === "string") {
      args[key] = item;
      continue;
    }
    if (
      Array.isArray(item) &&
      item.every((entry) => typeof entry === "string")
    ) {
      args[key] = item;
      continue;
    }
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be a string or array of strings`,
    );
  }
  return args;
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

  const workType: FactoryWorkType = {
    name: readRequiredString(record, "name", path),
    states: readRequiredArray(record, "states", path, decodeWorkState),
  };
  const id = readOptionalString(record, "id", path);
  const description = readOptionalObject(
    record,
    "description",
    path,
    decodeNameValue,
  );
  const handlingBehavior = readOptionalEnumArray(
    record,
    "handlingBehavior",
    path,
    WORK_TYPE_HANDLING_BEHAVIOR_VALUES,
  );
  if (id !== undefined) {
    workType.id = id;
  }
  if (description !== undefined) {
    workType.description = description;
  }
  if (handlingBehavior !== undefined) {
    workType.handlingBehavior = handlingBehavior;
  }
  return workType;
}

function decodeWorkState(value: unknown, path: string): FactoryWorkState {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORK_STATE_KEYS, path);

  const state: FactoryWorkState = {
    name: readRequiredString(record, "name", path),
    type: readRequiredEnum(record, "type", path, WORK_STATE_TYPE_VALUES),
  };
  const id = readOptionalString(record, "id", path);
  if (id !== undefined) {
    state.id = id;
  }
  return state;
}

function decodeResource(value: unknown, path: string): FactoryResource {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, RESOURCE_KEYS, path);

  const resource: FactoryResource = {
    capacity: readRequiredInteger(record, "capacity", path),
    name: readRequiredString(record, "name", path),
  };
  const id = readOptionalString(record, "id", path);
  if (id !== undefined) {
    resource.id = id;
  }
  return resource;
}

function decodeWorker(
  value: unknown,
  path: string,
  invocationSignature: FactoryInvocationSignature | undefined,
): FactoryWorker {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKER_KEYS, path);

  const worker: FactoryWorker = {
    name: readRequiredString(record, "name", path),
  };
  const id = readOptionalString(record, "id", path);
  const description = readOptionalObject(
    record,
    "description",
    path,
    decodeNameValue,
  );
  const type = readOptionalEnum(record, "type", path, WORKER_TYPE_VALUES);
  const model = readOptionalString(record, "model", path);
  const modelProvider = readOptionalWorkerModelProvider(
    record,
    path,
    invocationSignature,
  );
  const modelLocality = readOptionalEnum(
    record,
    "modelLocality",
    path,
    WORKER_MODEL_LOCALITY_VALUES,
  );
  const provider = readOptionalEnum(
    record,
    "provider",
    path,
    HOSTED_WORKER_PROVIDER_VALUES,
  );
  const executorProvider = readOptionalEnum(
    record,
    "executorProvider",
    path,
    WORKER_PROVIDER_VALUES,
  );
  const command = readOptionalString(record, "command", path);
  const args = readOptionalStringArray(record, "args", path);
  const resources = readOptionalArray(
    record,
    "resources",
    path,
    decodeResourceRequirement,
  );
  const timeout = readOptionalString(record, "timeout", path);
  const stopToken = readOptionalString(record, "stopToken", path);
  const skipPermissions = readOptionalBoolean(record, "skipPermissions", path);
  const auth = readOptionalObject(record, "auth", path, decodeHostedWorkerAuth);
  const linear = readOptionalObject(
    record,
    "linear",
    path,
    decodeHostedLinearWorkerConfig,
  );
  const body = readOptionalString(record, "body", path);
  const operations = readOptionalArray(
    record,
    "operations",
    path,
    decodeModelOperation,
  );

  if (id !== undefined) {
    worker.id = id;
  }
  if (description !== undefined) {
    worker.description = description;
  }
  if (type !== undefined) {
    worker.type = type;
  }
  if (model !== undefined) {
    worker.model = model;
  }
  if (modelProvider !== undefined) {
    worker.modelProvider = modelProvider;
  }
  if (modelLocality !== undefined) {
    worker.modelLocality = modelLocality;
  }
  if (provider !== undefined) {
    worker.provider = provider;
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
  if (auth !== undefined) {
    worker.auth = auth;
  }
  if (linear !== undefined) {
    worker.linear = linear;
  }
  if (body !== undefined) {
    worker.body = body;
  }
  if (operations !== undefined) {
    worker.operations = operations;
  }

  return worker;
}

function readOptionalWorkerModelProvider(
  value: Record<string, unknown>,
  path: string,
  invocationSignature: FactoryInvocationSignature | undefined,
): FactoryWorker["modelProvider"] | undefined {
  const modelProvider = readOptionalString(value, "modelProvider", path);
  if (modelProvider === undefined) {
    return undefined;
  }
  if (
    WORKER_MODEL_PROVIDER_VALUES.has(
      modelProvider as FactoryWorkerModelProvider,
    ) ||
    PROVIDER_IDENTITY_PATTERN.test(modelProvider)
  ) {
    return modelProvider as FactoryWorker["modelProvider"];
  }
  if (isDeclaredInvocationPlaceholder(modelProvider, invocationSignature)) {
    return modelProvider as FactoryWorker["modelProvider"];
  }
  throw new FactoryDefinitionAPIError(
    `${path}.modelProvider must be a valid provider identity or one of ${Array.from(WORKER_MODEL_PROVIDER_VALUES).join(", ")}.`,
  );
}

function isDeclaredInvocationPlaceholder(
  value: string,
  invocationSignature: FactoryInvocationSignature | undefined,
): boolean {
  if (!invocationSignature?.parameters?.length) {
    return false;
  }
  const match = EXACT_INVOCATION_PLACEHOLDER_PATTERN.exec(value.trim());
  if (!match) {
    return false;
  }
  const parameterName = match[1]?.trim();
  return invocationSignature.parameters.some(
    (parameter) => parameter.name.trim() === parameterName,
  );
}

function decodeFactoryLayout(value: unknown, path: string): FactoryLayout {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_KEYS, path);

  const layout: FactoryLayout = {
    schemaVersion: readRequiredInteger(record, "schemaVersion", path),
  };
  const nodes = readOptionalArray(
    record,
    "nodes",
    path,
    decodeFactoryLayoutNode,
  );
  const edges = readOptionalArray(
    record,
    "edges",
    path,
    decodeFactoryLayoutEdge,
  );
  const groups = readOptionalArray(
    record,
    "groups",
    path,
    decodeFactoryLayoutGroup,
  );
  const viewport = readOptionalObject(
    record,
    "viewport",
    path,
    decodeFactoryLayoutViewport,
  );
  const preferences = readOptionalObject(
    record,
    "preferences",
    path,
    decodeFactoryLayoutPreferences,
  );

  if (nodes !== undefined) {
    layout.nodes = nodes;
  }
  if (edges !== undefined) {
    layout.edges = edges;
  }
  if (groups !== undefined) {
    layout.groups = groups;
  }
  if (viewport !== undefined) {
    layout.viewport = viewport;
  }
  if (preferences !== undefined) {
    layout.preferences = preferences;
  }

  return layout;
}

function decodeFactoryLayoutNode(
  value: unknown,
  path: string,
): FactoryLayoutNode {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_NODE_KEYS, path);

  const node: FactoryLayoutNode = {
    id: readRequiredString(record, "id", path),
    position: decodeFactoryLayoutPointRequired(record, "position", path),
  };
  const size = readOptionalObject(
    record,
    "size",
    path,
    decodeFactoryLayoutSize,
  );
  const locked = readOptionalBoolean(record, "locked", path);
  if (size !== undefined) {
    node.size = size;
  }
  if (locked !== undefined) {
    node.locked = locked;
  }
  return node;
}

function decodeFactoryLayoutEdge(
  value: unknown,
  path: string,
): FactoryLayoutEdge {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_EDGE_KEYS, path);

  const edge: FactoryLayoutEdge = {
    id: readRequiredString(record, "id", path),
  };
  const waypoints = readOptionalArray(
    record,
    "waypoints",
    path,
    decodeFactoryLayoutPoint,
  );
  const labelPosition = readOptionalObject(
    record,
    "labelPosition",
    path,
    decodeFactoryLayoutPoint,
  );
  if (waypoints !== undefined) {
    edge.waypoints = waypoints;
  }
  if (labelPosition !== undefined) {
    edge.labelPosition = labelPosition;
  }
  return edge;
}

function decodeFactoryLayoutGroup(
  value: unknown,
  path: string,
): FactoryLayoutGroup {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_GROUP_KEYS, path);

  const group: FactoryLayoutGroup = {
    bounds: decodeFactoryLayoutBoundsRequired(record, "bounds", path),
    id: readRequiredString(record, "id", path),
    nodeIds: readRequiredStringArray(record, "nodeIds", path),
  };
  const label = readOptionalString(record, "label", path);
  const parentGroupId = readOptionalNullableString(
    record,
    "parentGroupId",
    path,
  );
  const color = readOptionalString(record, "color", path);
  const locked = readOptionalBoolean(record, "locked", path);
  if (label !== undefined) {
    group.label = label;
  }
  if (parentGroupId !== undefined) {
    group.parentGroupId = parentGroupId;
  }
  if (color !== undefined) {
    group.color = color;
  }
  if (locked !== undefined) {
    group.locked = locked;
  }
  return group;
}

function decodeFactoryLayoutPreferences(
  value: unknown,
  path: string,
): FactoryLayoutPreferences {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_PREFERENCES_KEYS, path);

  const preferences: FactoryLayoutPreferences = {};
  const direction = readOptionalEnum(
    record,
    "direction",
    path,
    LAYOUT_PREFERENCE_DIRECTION_VALUES,
  );
  if (direction !== undefined) {
    preferences.direction = direction as FactoryLayoutPreferences["direction"];
  }
  return preferences;
}

function decodeFactoryLayoutViewport(
  value: unknown,
  path: string,
): FactoryLayoutViewport {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_VIEWPORT_KEYS, path);

  return {
    x: readRequiredNumber(record, "x", path),
    y: readRequiredNumber(record, "y", path),
    zoom: readRequiredNumber(record, "zoom", path),
  };
}

function decodeFactoryLayoutPoint(
  value: unknown,
  path: string,
): FactoryLayoutPoint {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_POINT_KEYS, path);

  return {
    x: readRequiredNumber(record, "x", path),
    y: readRequiredNumber(record, "y", path),
  };
}

function decodeFactoryLayoutPointRequired(
  value: Record<string, unknown>,
  key: string,
  path: string,
): FactoryLayoutPoint {
  const item = value[key];
  if (item === undefined || item === null) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return decodeFactoryLayoutPoint(item, `${path}.${key}`);
}

function decodeFactoryLayoutSize(
  value: unknown,
  path: string,
): FactoryLayoutNode["size"] {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_SIZE_KEYS, path);

  return {
    height: readRequiredNumber(record, "height", path),
    width: readRequiredNumber(record, "width", path),
  };
}

function decodeFactoryLayoutBoundsRequired(
  value: Record<string, unknown>,
  key: string,
  path: string,
): FactoryLayoutBounds {
  const item = value[key];
  if (item === undefined || item === null) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return decodeFactoryLayoutBounds(item, `${path}.${key}`);
}

function decodeFactoryLayoutBounds(
  value: unknown,
  path: string,
): FactoryLayoutBounds {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, LAYOUT_BOUNDS_KEYS, path);

  return {
    height: readRequiredNumber(record, "height", path),
    width: readRequiredNumber(record, "width", path),
    x: readRequiredNumber(record, "x", path),
    y: readRequiredNumber(record, "y", path),
  };
}

function decodeHostedWorkerAuth(
  value: unknown,
  path: string,
): FactoryHostedWorkerAuth {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, HOSTED_WORKER_AUTH_KEYS, path);

  const auth: FactoryHostedWorkerAuth = {};
  const secretRef = readOptionalString(record, "secretRef", path);
  if (secretRef !== undefined) {
    auth.secretRef = secretRef;
  }
  return auth;
}

function decodeHostedLinearWorkerConfig(
  value: unknown,
  path: string,
): FactoryHostedLinearWorkerConfig {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, HOSTED_LINEAR_WORKER_KEYS, path);

  const config: FactoryHostedLinearWorkerConfig = {};
  const pollInterval = readOptionalString(record, "pollInterval", path);
  const teamIds = readOptionalStringArray(record, "teamIds", path);
  const stateIds = readOptionalStringArray(record, "stateIds", path);
  const mapping = readOptionalObject(
    record,
    "mapping",
    path,
    decodeHostedLinearWorkerMapping,
  );
  const claim = readOptionalObject(
    record,
    "claim",
    path,
    decodeHostedLinearWorkerClaim,
  );

  if (pollInterval !== undefined) {
    config.pollInterval = pollInterval;
  }
  if (teamIds !== undefined) {
    config.teamIds = teamIds;
  }
  if (stateIds !== undefined) {
    config.stateIds = stateIds;
  }
  if (mapping !== undefined) {
    config.mapping = mapping;
  }
  if (claim !== undefined) {
    config.claim = claim;
  }
  return config;
}

function decodeHostedLinearWorkerMapping(
  value: unknown,
  path: string,
): FactoryHostedLinearWorkerMapping {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, HOSTED_LINEAR_WORKER_MAPPING_KEYS, path);

  const mapping: FactoryHostedLinearWorkerMapping = {};
  const workType = readOptionalString(record, "workType", path);
  const state = readOptionalString(record, "state", path);
  if (workType !== undefined) {
    mapping.workType = workType;
  }
  if (state !== undefined) {
    mapping.state = state;
  }
  return mapping;
}

function decodeHostedLinearWorkerClaim(
  value: unknown,
  path: string,
): FactoryHostedLinearWorkerClaim {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, HOSTED_LINEAR_WORKER_CLAIM_KEYS, path);

  const claim: FactoryHostedLinearWorkerClaim = {};
  const assigneeField = readOptionalString(record, "assigneeField", path);
  if (assigneeField !== undefined) {
    claim.assigneeField = assigneeField;
  }
  return claim;
}

function decodeWorkstation(value: unknown, path: string): FactoryWorkstation {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_KEYS, path);

  const workstation: FactoryWorkstation = {
    inputs: readRequiredArray(record, "inputs", path, decodeWorkstationIO),
    name: readRequiredString(record, "name", path),
    worker: readRequiredString(record, "worker", path),
  };
  const id = readOptionalString(record, "id", path);
  const description = readOptionalObject(
    record,
    "description",
    path,
    decodeNameValue,
  );
  const behavior = readOptionalEnum(
    record,
    "behavior",
    path,
    WORKSTATION_BEHAVIOR_VALUES,
  );
  const type = readOptionalEnum(record, "type", path, WORKSTATION_TYPE_VALUES);
  const promptFile = readOptionalString(record, "promptFile", path);
  const outputSchema = readOptionalString(record, "outputSchema", path);
  const limits = readOptionalObject(
    record,
    "limits",
    path,
    decodeWorkstationLimits,
  );
  const body = readOptionalString(record, "body", path);
  const cron = readOptionalObject(record, "cron", path, decodeWorkstationCron);
  const outputs = readOptionalArray(
    record,
    "outputs",
    path,
    decodeWorkstationIO,
  );
  const classificationRoutes = readOptionalArray(
    record,
    "classificationRoutes",
    path,
    decodeClassificationRoute,
  );
  const onContinue = readOptionalArray(
    record,
    "onContinue",
    path,
    decodeWorkstationIO,
  );
  const onRejection = readOptionalArray(
    record,
    "onRejection",
    path,
    decodeWorkstationIO,
  );
  const onFailure = readOptionalArray(
    record,
    "onFailure",
    path,
    decodeWorkstationIO,
  );
  const resources = readOptionalArray(
    record,
    "resources",
    path,
    decodeResourceRequirement,
  );
  const copyReferencedScripts = readOptionalBoolean(
    record,
    "copyReferencedScripts",
    path,
  );
  const guards = readOptionalArray(
    record,
    "guards",
    path,
    decodeWorkstationGuard,
  );
  const stopWords = readOptionalStringArray(record, "stopWords", path);
  const workingDirectory = readOptionalString(record, "workingDirectory", path);
  const worktree = readOptionalString(record, "worktree", path);
  const env = readOptionalStringMap(record, "env", path);
  const runner = readOptionalEnum(record, "runner", path, RUNNER_ID_VALUES);
  const operation = readOptionalString(record, "operation", path);
  const operationBindings = readOptionalArray(
    record,
    "operationBindings",
    path,
    decodeWorkstationOperationBinding,
  );

  if (id !== undefined) {
    workstation.id = id;
  }
  if (description !== undefined) {
    workstation.description = description;
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
  if (cron !== undefined) {
    workstation.cron = cron;
  }
  if (outputs !== undefined) {
    workstation.outputs = outputs;
  }
  if (classificationRoutes !== undefined) {
    workstation.classificationRoutes = classificationRoutes;
  }
  if (onContinue !== undefined) {
    workstation.onContinue = onContinue;
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
  if (runner !== undefined) {
    workstation.runner = runner;
  }
  if (operation !== undefined) {
    workstation.operation = operation;
  }
  if (operationBindings !== undefined) {
    workstation.operationBindings = operationBindings;
  }

  return workstation;
}

function decodeClassificationRoute(
  value: unknown,
  path: string,
): FactoryClassificationRoute {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, CLASSIFICATION_ROUTE_KEYS, path);

  return {
    label: readRequiredString(record, "label", path),
    outputs: readRequiredArray(record, "outputs", path, decodeWorkstationIO),
  };
}

function decodeWorkstationIO(
  value: unknown,
  path: string,
): FactoryWorkstationIO {
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
    type: readRequiredEnum(
      record,
      "type",
      path,
      FACTORY_ROOT_GUARD_TYPE_VALUES,
    ),
    modelProvider: readRequiredEnum(
      record,
      "modelProvider",
      path,
      WORKER_MODEL_PROVIDER_VALUES,
    ),
    refreshWindow: readRequiredString(record, "refreshWindow", path),
  };
  const model = readOptionalString(record, "model", path);
  if (model !== undefined) {
    guard.model = model;
  }
  return guard;
}

function decodeWorkstationGuard(
  value: unknown,
  path: string,
): FactoryWorkstationGuard {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, GUARD_KEYS, path);

  const guard: FactoryWorkstationGuard = {
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

function decodeInputGuard(value: unknown, path: string): FactoryInputGuard {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, GUARD_KEYS, path);

  const guard: FactoryInputGuard = {
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
): FactoryWorkstationGuard["matchConfig"] | undefined {
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

function decodeWorkstationLimits(
  value: unknown,
  path: string,
): FactoryWorkstationLimits {
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

function decodeWorkstationCron(
  value: unknown,
  path: string,
): FactoryWorkstationCron {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKSTATION_CRON_KEYS, path);

  const cron: FactoryWorkstationCron = {
    schedule: readRequiredString(record, "schedule", path),
    triggerAtStart:
      readOptionalBoolean(record, "triggerAtStart", path) ?? false,
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

function decodeResourceRequirement(
  value: unknown,
  path: string,
): FactoryResourceRequirement {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, RESOURCE_REQUIREMENT_KEYS, path);

  return {
    capacity: readRequiredInteger(record, "capacity", path),
    name: readRequiredString(record, "name", path),
  };
}
