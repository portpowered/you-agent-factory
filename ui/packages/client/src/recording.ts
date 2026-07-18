import {
  FACTORY_EVENT_TYPES,
  type FactoryDefinition,
  type FactoryEvent,
} from "./contracts.js";

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
  | "unsupported_event_type";

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

  return issues.length > 0
    ? { success: false, issues }
    : { success: true, data: input as unknown as FactoryRecording };
}

export function parseFactoryRecording(input: unknown): FactoryRecording {
  const result = safeParseFactoryRecording(input);
  if (!result.success) {
    throw new FactoryRecordingValidationError(result.issues);
  }
  return result.data;
}
