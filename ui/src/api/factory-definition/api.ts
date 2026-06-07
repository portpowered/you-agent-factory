/**
 * Canonical factory-definition normalization for the dashboard API boundary.
 *
 * This module shapes and validates `Factory` payloads against the generated OpenAPI
 * contract. It performs no HTTP, no `fetch`, and does not import `transport.ts`.
 * Session-scoped factory GET/PUT lives in `ui/src/api/session-factory/`; editor and
 * import adapters delegate there after normalizing documents through this module.
 */
import type { components } from "../generated/openapi";

export type CanonicalFactoryDefinition = components["schemas"]["Factory"];

type FactorySchemas = components["schemas"];
type FactoryRootGuard = FactorySchemas["FactoryGuard"];
type FactoryGuard = FactorySchemas["Guard"];
type FactoryHostedLinearWorkerClaim = FactorySchemas["HostedLinearWorkerClaim"];
type FactoryHostedLinearWorkerConfig =
  FactorySchemas["HostedLinearWorkerConfig"];
type FactoryHostedLinearWorkerMapping =
  FactorySchemas["HostedLinearWorkerMapping"];
type FactoryHostedWorkerAuth = FactorySchemas["HostedWorkerAuth"];
type FactoryInputType = FactorySchemas["InputType"];
type FactoryResource = FactorySchemas["Resource"];
type FactoryResourceRequirement = FactorySchemas["ResourceRequirement"];
type FactoryRunnerID = FactorySchemas["RunnerID"];
type FactoryVersion = FactorySchemas["HybridLogicalTimestamp"];
type FactoryLayout = FactorySchemas["FactoryLayout"];
type FactoryLayoutBounds = FactorySchemas["FactoryLayoutBounds"];
type FactoryLayoutEdge = FactorySchemas["FactoryLayoutEdge"];
type FactoryLayoutGroup = FactorySchemas["FactoryLayoutGroup"];
type FactoryLayoutNode = FactorySchemas["FactoryLayoutNode"];
type FactoryLayoutPoint = FactorySchemas["FactoryLayoutPoint"];
type FactoryLayoutPreferences = FactorySchemas["FactoryLayoutPreferences"];
type FactoryLayoutViewport = FactorySchemas["FactoryLayoutViewport"];
type FactoryWorker = FactorySchemas["Worker"];
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
  "factoryDirectory",
  "guards",
  "id",
  "inputTypes",
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
const INPUT_TYPE_KEYS = new Set(["name", "type"]);
const WORK_TYPE_KEYS = new Set(["handlingBehavior", "id", "name", "states"]);
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
  "executorProvider",
  "linear",
  "model",
  "modelLocality",
  "modelProvider",
  "name",
  "id",
  "provider",
  "resources",
  "openCodeAgent",
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
  "openCodeAgent",
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
const WORK_STATE_TYPE_VALUES = new Set<FactoryWorkState["type"]>([
  "FAILED",
  "INITIAL",
  "PROCESSING",
  "TERMINAL",
]);
const WORKER_TYPE_VALUES = new Set<NonNullable<FactoryWorker["type"]>>([
  "HOSTED_WORKER",
  "MODEL_WORKER",
  "SCRIPT_WORKER",
]);
const WORKER_MODEL_PROVIDER_VALUES = new Set<
  NonNullable<FactoryWorker["modelProvider"]>
>(["CLAUDE", "CODEX", "CURSOR", "GEMINI", "KIRO", "OPENCODE"]);
const WORKER_PROVIDER_VALUES = new Set<
  NonNullable<FactoryWorker["executorProvider"]>
>(["SCRIPT_WRAP"]);
const WORKER_MODEL_LOCALITY_VALUES = new Set<
  NonNullable<FactoryWorker["modelLocality"]>
>(["LOCAL", "CLOUD"]);
const HOSTED_WORKER_PROVIDER_VALUES = new Set<
  NonNullable<FactoryWorker["provider"]>
>(["LINEAR"]);
const RUNNER_ID_VALUES = new Set<FactoryRunnerID>([
  "codex",
  "gemini",
  "kiro",
  "cursor-cli",
  "opencode",
]);
const WORKSTATION_BEHAVIOR_VALUES = new Set<
  NonNullable<FactoryWorkstation["behavior"]>
>(["CRON", "POLLER", "REPEATER", "STANDARD"]);
const WORKSTATION_TYPE_VALUES = new Set<
  NonNullable<FactoryWorkstation["type"]>
>(["CLASSIFIER_WORKSTATION", "LOGICAL_MOVE", "MODEL_WORKSTATION"]);
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
  "SAME_TRACE_ID",
]);

export class FactoryDefinitionAPIError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = "FactoryDefinitionAPIError";
  }
}

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
  const runner = readOptionalEnum(value, "runner", path, RUNNER_ID_VALUES);
  const supportingFiles = readOptionalObject(
    value,
    "supportingFiles",
    path,
    expectObject,
  );
  const version = readOptionalFactoryVersion(value, "version", path);
  const workers = readOptionalArray(value, "workers", path, decodeWorker);
  const workstations = readOptionalArray(
    value,
    "workstations",
    path,
    decodeWorkstation,
  );

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
  const handlingBehavior = readOptionalEnumArray(
    record,
    "handlingBehavior",
    path,
    WORK_TYPE_HANDLING_BEHAVIOR_VALUES,
  );
  if (id !== undefined) {
    workType.id = id;
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

function decodeWorker(value: unknown, path: string): FactoryWorker {
  const record = expectObject(value, path);
  rejectUnknownKeys(record, WORKER_KEYS, path);

  const worker: FactoryWorker = {
    name: readRequiredString(record, "name", path),
  };
  const id = readOptionalString(record, "id", path);
  const type = readOptionalEnum(record, "type", path, WORKER_TYPE_VALUES);
  const model = readOptionalString(record, "model", path);
  const modelProvider = readOptionalEnum(
    record,
    "modelProvider",
    path,
    WORKER_MODEL_PROVIDER_VALUES,
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
  const openCodeAgent = readOptionalNonEmptyString(
    record,
    "openCodeAgent",
    path,
  );
  const auth = readOptionalObject(record, "auth", path, decodeHostedWorkerAuth);
  const linear = readOptionalObject(
    record,
    "linear",
    path,
    decodeHostedLinearWorkerConfig,
  );
  const body = readOptionalString(record, "body", path);

  if (id !== undefined) {
    worker.id = id;
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
  if (openCodeAgent !== undefined) {
    worker.openCodeAgent = openCodeAgent;
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

  return worker;
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
  const openCodeAgent = readOptionalNonEmptyString(
    record,
    "openCodeAgent",
    path,
  );

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
  if (openCodeAgent !== undefined) {
    workstation.openCodeAgent = openCodeAgent;
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

function readOptionalNonEmptyString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string | undefined {
  const item = readOptionalString(value, key, path);
  if (item === undefined) {
    return undefined;
  }
  if (item.trim() === "") {
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be a non-empty string.`,
    );
  }
  return item;
}

function readRequiredString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string {
  const item = readOptionalString(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

function readOptionalNullableString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string | null | undefined {
  const item = value[key];
  if (item === undefined) {
    return undefined;
  }
  if (item === null) {
    return null;
  }
  if (typeof item !== "string") {
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be a string or null.`,
    );
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

function readRequiredInteger(
  value: Record<string, unknown>,
  key: string,
  path: string,
): number {
  const item = readOptionalInteger(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

function readRequiredNumber(
  value: Record<string, unknown>,
  key: string,
  path: string,
): number {
  const item = value[key];
  if (item === undefined || item === null) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  if (typeof item !== "number" || !Number.isFinite(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be a number.`);
  }
  return item;
}

function readOptionalEnumArray<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T[] | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (!Array.isArray(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an array.`);
  }
  return item.map((entry, index) => {
    const entryPath = `${path}.${key}[${index}]`;
    if (typeof entry !== "string") {
      throw new FactoryDefinitionAPIError(`${entryPath} must be a string.`);
    }
    if (!allowedValues.has(entry as T)) {
      throw new FactoryDefinitionAPIError(
        `${entryPath} must be one of ${Array.from(allowedValues).join(", ")}.`,
      );
    }
    return entry as T;
  });
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
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be an array of strings.`,
    );
  }
  return item.map((entry, index) => {
    if (typeof entry !== "string") {
      throw new FactoryDefinitionAPIError(
        `${path}.${key}[${index}] must be a string.`,
      );
    }
    return entry;
  });
}

function readRequiredStringArray(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string[] {
  const item = readOptionalStringArray(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
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
      throw new FactoryDefinitionAPIError(
        `${path}.${key}.${mapKey} must be a string.`,
      );
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

function readOptionalFactoryVersion(
  value: Record<string, unknown>,
  key: string,
  path: string,
): FactoryVersion | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  const record = expectObject(item, `${path}.${key}`);
  const logical = record.logical;
  const physical = record.physical;
  if (!isFactoryVersionLogicalValue(logical)) {
    throw new FactoryDefinitionAPIError(
      `${path}.${key}.logical must be a decimal string.`,
    );
  }
  if (typeof physical !== "string") {
    throw new FactoryDefinitionAPIError(
      `${path}.${key}.physical must be a string.`,
    );
  }
  return {
    logical: String(logical),
    physical,
  };
}

function isFactoryVersionLogicalValue(
  value: unknown,
): value is number | string {
  if (typeof value === "string") {
    return /^[0-9]+$/.test(value);
  }
  return typeof value === "number" && Number.isFinite(value);
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
