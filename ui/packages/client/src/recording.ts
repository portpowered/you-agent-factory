import {
  FACTORY_EVENT_SCHEMA_VERSIONS,
  FACTORY_EVENT_TYPES,
  type FactoryDefinition,
  type FactoryEvent,
} from "./contracts.js";
import { orderFactoryEvents } from "./event-ordering.js";

export const FACTORY_RECORDING_SCHEMA_VERSION = "factory-recording/v1" as const;

export interface FactoryRecording {
  schemaVersion: typeof FACTORY_RECORDING_SCHEMA_VERSION;
  id: string;
  title: string;
  summary?: string;
  factory?: FactoryDefinition;
  events: FactoryEvent[];
}

export type RecordingValidationIssueCode =
  | "invalid_type"
  | "missing_required_field"
  | "unsupported_recording_schema_version"
  | "unsupported_event_type"
  | "unsupported_event_schema_version"
  | "duplicate_event_id"
  | "mixed_factory_session_identity"
  | "missing_topology_bootstrap";

export interface RecordingValidationIssue {
  category: "structure" | "semantic";
  code: RecordingValidationIssueCode;
  path: readonly (string | number)[];
  message: string;
}

export type SafeParseFactoryRecordingResult =
  | { success: true; data: FactoryRecording }
  | { success: false; issues: readonly RecordingValidationIssue[] };

export class FactoryRecordingValidationError extends Error {
  readonly issues: readonly RecordingValidationIssue[];

  constructor(issues: readonly RecordingValidationIssue[]) {
    super(
      issues.length === 1
        ? `Factory recording validation failed: ${issues[0]?.message}`
        : `Factory recording validation failed with ${issues.length} issues`,
    );
    this.name = "FactoryRecordingValidationError";
    this.issues = issues;
  }
}

type InputRecord = Record<string, unknown>;

const supportedEventTypes = new Set<string>(Object.values(FACTORY_EVENT_TYPES));
const supportedEventSchemaVersions = new Set<string>(
  Object.values(FACTORY_EVENT_SCHEMA_VERSIONS),
);
const topologyEventTypes = new Set<string>([
  FACTORY_EVENT_TYPES.FactoryEventTypeRunRequest,
  FACTORY_EVENT_TYPES.FactoryEventTypeInitialStructureRequest,
  FACTORY_EVENT_TYPES.FactoryEventTypeFactoryChange,
]);

function isRecord(value: unknown): value is InputRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function addRequiredStringIssue(
  value: InputRecord,
  key: string,
  path: readonly (string | number)[],
  issues: RecordingValidationIssue[],
): void {
  if (!(key in value)) {
    issues.push({
      category: "structure",
      code: "missing_required_field",
      path: [...path, key],
      message: `Expected required field ${key}.`,
    });
  } else if (typeof value[key] !== "string") {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: [...path, key],
      message: `Expected ${key} to be a string.`,
    });
  }
}

function addRequiredNumberIssue(
  value: InputRecord,
  key: string,
  path: readonly (string | number)[],
  issues: RecordingValidationIssue[],
): void {
  if (!(key in value)) {
    issues.push({
      category: "structure",
      code: "missing_required_field",
      path: [...path, key],
      message: `Expected required field ${key}.`,
    });
  } else if (typeof value[key] !== "number" || !Number.isFinite(value[key])) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: [...path, key],
      message: `Expected ${key} to be a finite number.`,
    });
  }
}

function validateFactoryDefinition(
  input: unknown,
  path: readonly (string | number)[],
  issues: RecordingValidationIssue[],
): void {
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected factory to be an object.",
    });
    return;
  }

  addRequiredStringIssue(input, "name", path, issues);
}

/** Validate the shared canonical event envelope without applying recording-wide rules. */
export function validateFactoryEventEnvelope(
  input: unknown,
  path: readonly (string | number)[] = [],
): RecordingValidationIssue[] {
  const issues: RecordingValidationIssue[] = [];
  if (!isRecord(input)) {
    return [
      {
        category: "structure",
        code: "invalid_type",
        path,
        message: "Expected Factory event to be an object.",
      },
    ];
  }

  addRequiredStringIssue(input, "schemaVersion", path, issues);
  addRequiredStringIssue(input, "id", path, issues);
  addRequiredStringIssue(input, "type", path, issues);

  if (typeof input.type === "string" && !supportedEventTypes.has(input.type)) {
    issues.push({
      category: "structure",
      code: "unsupported_event_type",
      path: [...path, "type"],
      message: `Unsupported Factory event type: ${input.type}.`,
    });
  }

  if (!isRecord(input.context)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: [...path, "context"],
      message: "Expected event context to be an object.",
    });
  } else {
    addRequiredNumberIssue(
      input.context,
      "sequence",
      [...path, "context"],
      issues,
    );
    addRequiredNumberIssue(input.context, "tick", [...path, "context"], issues);
    addRequiredStringIssue(
      input.context,
      "eventTime",
      [...path, "context"],
      issues,
    );
    if (
      input.context.sessionSequence !== undefined &&
      (typeof input.context.sessionSequence !== "number" ||
        !Number.isFinite(input.context.sessionSequence))
    ) {
      issues.push({
        category: "structure",
        code: "invalid_type",
        path: [...path, "context", "sessionSequence"],
        message: "Expected sessionSequence to be a finite number when present.",
      });
    }
  }

  if (!isRecord(input.payload)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: [...path, "payload"],
      message: "Expected event payload to be an object.",
    });
  } else if (
    input.type === FACTORY_EVENT_TYPES.FactoryEventTypeRunRequest ||
    input.type ===
      FACTORY_EVENT_TYPES.FactoryEventTypeInitialStructureRequest ||
    input.type === FACTORY_EVENT_TYPES.FactoryEventTypeFactoryChange
  ) {
    validateFactoryDefinition(
      input.payload.factory,
      [...path, "payload", "factory"],
      issues,
    );
  }

  return issues;
}

function hasUsableFactoryDefinition(input: unknown): boolean {
  return (
    isRecord(input) &&
    typeof input.name === "string" &&
    input.name.trim().length > 0
  );
}

function validateEventSchemaVersions(
  events: readonly InputRecord[],
  issues: RecordingValidationIssue[],
): void {
  for (const [index, event] of events.entries()) {
    issues.push(...validateFactoryEventSchemaVersion(event, ["events", index]));
  }
}

/** Validate the generated Factory event schema version for any input source. */
export function validateFactoryEventSchemaVersion(
  input: unknown,
  path: readonly (string | number)[] = [],
): RecordingValidationIssue[] {
  if (
    !isRecord(input) ||
    typeof input.schemaVersion !== "string" ||
    supportedEventSchemaVersions.has(input.schemaVersion)
  ) {
    return [];
  }

  return [
    {
      category: "semantic",
      code: "unsupported_event_schema_version",
      path: [...path, "schemaVersion"],
      message: `Unsupported Factory event schema version: ${input.schemaVersion}.`,
    },
  ];
}

function validateUniqueEventIds(
  events: readonly InputRecord[],
  issues: RecordingValidationIssue[],
): void {
  const locationsById = new Map<string, number[]>();
  for (const [index, event] of events.entries()) {
    const locations = locationsById.get(event.id as string) ?? [];
    locations.push(index);
    locationsById.set(event.id as string, locations);
  }

  for (const [eventId, locations] of locationsById) {
    if (locations.length < 2) {
      continue;
    }
    const locationList = locations.join(", ");
    for (const index of locations) {
      issues.push({
        category: "semantic",
        code: "duplicate_event_id",
        path: ["events", index, "id"],
        message: `Event ID ${eventId} is duplicated at event indexes ${locationList}.`,
      });
    }
  }
}

function validateFactorySessionIdentity(
  events: readonly InputRecord[],
  issues: RecordingValidationIssue[],
): void {
  const sessionIds = events.map((event) =>
    isRecord(event.context) && typeof event.context.sessionId === "string"
      ? event.context.sessionId
      : undefined,
  );
  const presentSessionIds = new Set(
    sessionIds.filter(
      (sessionId): sessionId is string => sessionId !== undefined,
    ),
  );
  const identitiesAreMixed =
    presentSessionIds.size > 1 ||
    (presentSessionIds.size === 1 && sessionIds.some((id) => id === undefined));

  if (!identitiesAreMixed) {
    return;
  }
  for (const [index, sessionId] of sessionIds.entries()) {
    issues.push({
      category: "semantic",
      code: "mixed_factory_session_identity",
      path: ["events", index, "context", "sessionId"],
      message:
        sessionId === undefined
          ? "Expected a Factory Session ID consistent with the other recording events."
          : `Factory Session ID ${sessionId} is inconsistent with the recording event history.`,
    });
  }
}

function validateTopologyBootstrap(
  factory: unknown,
  events: readonly InputRecord[],
  issues: RecordingValidationIssue[],
): void {
  const hasTopologyEvent = events.some(
    (event) =>
      typeof event.type === "string" &&
      topologyEventTypes.has(event.type) &&
      isRecord(event.payload) &&
      hasUsableFactoryDefinition(event.payload.factory),
  );

  if (!hasUsableFactoryDefinition(factory) && !hasTopologyEvent) {
    issues.push({
      category: "semantic",
      code: "missing_topology_bootstrap",
      path: ["events"],
      message:
        "Expected a usable top-level factory or a topology-establishing Factory event.",
    });
  }
}

function validateRecordingSemantics(
  input: InputRecord,
  events: readonly InputRecord[],
): RecordingValidationIssue[] {
  const issues: RecordingValidationIssue[] = [];
  validateEventSchemaVersions(events, issues);
  validateUniqueEventIds(events, issues);
  validateFactorySessionIdentity(events, issues);
  validateTopologyBootstrap(input.factory, events, issues);
  return issues;
}

export function safeParseFactoryRecording(
  input: unknown,
): SafeParseFactoryRecordingResult {
  if (!isRecord(input)) {
    return {
      success: false,
      issues: [
        {
          category: "structure",
          code: "invalid_type",
          path: [],
          message: "Expected Factory recording to be an object.",
        },
      ],
    };
  }

  const issues: RecordingValidationIssue[] = [];
  addRequiredStringIssue(input, "schemaVersion", [], issues);
  addRequiredStringIssue(input, "id", [], issues);
  addRequiredStringIssue(input, "title", [], issues);

  if (
    typeof input.schemaVersion === "string" &&
    input.schemaVersion !== FACTORY_RECORDING_SCHEMA_VERSION
  ) {
    issues.push({
      category: "structure",
      code: "unsupported_recording_schema_version",
      path: ["schemaVersion"],
      message: `Unsupported Factory recording schema version: ${input.schemaVersion}.`,
    });
  }

  if (input.summary !== undefined && typeof input.summary !== "string") {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: ["summary"],
      message: "Expected summary to be a string when present.",
    });
  }

  if (input.factory !== undefined) {
    validateFactoryDefinition(input.factory, ["factory"], issues);
  }

  if (!Array.isArray(input.events)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: ["events"],
      message: "Expected events to be an array.",
    });
  } else {
    for (const [index, event] of input.events.entries()) {
      issues.push(...validateFactoryEventEnvelope(event, ["events", index]));
    }
  }

  if (issues.length === 0 && Array.isArray(input.events)) {
    issues.push(
      ...validateRecordingSemantics(input, input.events as InputRecord[]),
    );
  }

  return issues.length > 0
    ? { success: false, issues }
    : {
        success: true,
        data: {
          ...(input as unknown as FactoryRecording),
          events: orderFactoryEvents(input.events as FactoryEvent[]),
        },
      };
}

export function parseFactoryRecording(input: unknown): FactoryRecording {
  const result = safeParseFactoryRecording(input);
  if (!result.success) {
    throw new FactoryRecordingValidationError(result.issues);
  }
  return result.data;
}
