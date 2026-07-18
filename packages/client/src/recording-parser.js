import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import recordingSchema from "./generated/factory-recording.schema.json" with {
  type: "json",
};

const TOPOLOGY_EVENT_TYPE = "INITIAL_STRUCTURE_REQUEST";

// The standalone schema intentionally retains its OpenAPI discriminator as a
// raw-data annotation. Mutation-affecting Ajv options stay explicitly disabled.
const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
addFormats(ajv);
const validateShape = ajv.compile(recordingSchema);

export class FactoryRecordingValidationError extends Error {
  constructor(issues) {
    super(`Invalid Factory Recording: ${issues.length} validation issue(s)`);
    this.name = "FactoryRecordingValidationError";
    this.issues = Object.freeze([...issues]);
  }
}

export function safeParseFactoryRecording(input) {
  if (!validateShape(input)) {
    return failure(shapeIssues(validateShape.errors ?? []));
  }

  const issues = semanticIssues(input);
  return issues.length === 0 ? { success: true, data: input } : failure(issues);
}

export function parseFactoryRecording(input) {
  const result = safeParseFactoryRecording(input);
  if (!result.success) {
    throw result.error;
  }
  return result.data;
}

function failure(issues) {
  const error = new FactoryRecordingValidationError(issues);
  return { success: false, error, issues: error.issues };
}

function shapeIssues(errors) {
  return errors.map((error) => {
    const path = issuePath(error);
    const eventIndex = eventIndexFromPath(path);
    const versionCode = versionIssueCode(error);
    return {
      code: versionCode ?? "INVALID_SHAPE",
      path,
      message: versionCode
        ? `Unsupported schema version at ${path}.`
        : `${path} ${error.message ?? "does not match the Factory Recording schema"}.`,
      ...(eventIndex === undefined ? {} : { eventIndex }),
    };
  });
}

function issuePath(error) {
  const property =
    error.keyword === "required"
      ? error.params.missingProperty
      : error.keyword === "additionalProperties"
        ? error.params.additionalProperty
        : undefined;
  if (typeof property !== "string") {
    return error.instancePath || "/";
  }
  return `${error.instancePath}/${property.replaceAll("~", "~0").replaceAll("/", "~1")}`;
}

function versionIssueCode(error) {
  if (error.keyword !== "enum") {
    return undefined;
  }
  if (error.instancePath === "/schemaVersion") {
    return "UNSUPPORTED_RECORDING_VERSION";
  }
  return /^\/events\/\d+\/schemaVersion$/.test(error.instancePath)
    ? "UNSUPPORTED_EVENT_VERSION"
    : undefined;
}

function eventIndexFromPath(path) {
  const match = /^\/events\/(\d+)(?:\/|$)/.exec(path);
  return match ? Number(match[1]) : undefined;
}

function semanticIssues(recording) {
  const issues = [];
  const eventIds = new Set();
  let hasTopology = false;

  for (const [eventIndex, event] of recording.events.entries()) {
    const location = { eventIndex, eventId: event.id };
    if (eventIds.has(event.id)) {
      issues.push({
        code: "DUPLICATE_EVENT_ID",
        path: `/events/${eventIndex}/id`,
        message: `Event id ${JSON.stringify(event.id)} appears more than once.`,
        ...location,
      });
    }
    eventIds.add(event.id);

    if (event.context.sessionId !== recording.sessionId) {
      issues.push({
        code: "MIXED_SESSION_ID",
        path: `/events/${eventIndex}/context/sessionId`,
        message: `Event sessionId must equal recording sessionId ${JSON.stringify(recording.sessionId)}.`,
        ...location,
      });
    }

    if (event.type === TOPOLOGY_EVENT_TYPE) {
      hasTopology = true;
    }

    if (
      eventIndex > 0 &&
      compareEvents(recording.events[eventIndex - 1], event) > 0
    ) {
      issues.push({
        code: "NON_CANONICAL_ORDER",
        path: `/events/${eventIndex}`,
        message:
          "Event is out of canonical tick, sequence, eventTime, and id order.",
        ...location,
      });
    }
  }

  if (!hasTopology) {
    issues.push({
      code: "MISSING_TOPOLOGY_BOOTSTRAP",
      path: "/events",
      message: `Recording must contain an ${TOPOLOGY_EVENT_TYPE} event.`,
    });
  }

  return issues;
}

function compareEvents(left, right) {
  return (
    compareNumber(left.context.tick, right.context.tick) ||
    compareNumber(left.context.sequence, right.context.sequence) ||
    compareEventTime(left.context.eventTime, right.context.eventTime) ||
    left.id.localeCompare(right.id)
  );
}

function compareEventTime(left, right) {
  const leftInstant = parseEventTime(left);
  const rightInstant = parseEventTime(right);
  return (
    compareNumber(leftInstant.wholeSecond, rightInstant.wholeSecond) ||
    compareFraction(leftInstant.fraction, rightInstant.fraction)
  );
}

function parseEventTime(value) {
  const match =
    /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/i.exec(
      value,
    );
  // Shape validation, including RFC 3339 format validation, runs before this
  // semantic comparison. Keep this guard explicit if that boundary changes.
  if (!match) {
    throw new TypeError(`Validated eventTime is not RFC 3339: ${value}`);
  }
  return {
    wholeSecond: Date.parse(`${match[1]}${match[3]}`),
    fraction: match[2] ?? "",
  };
}

function compareFraction(left, right) {
  const width = Math.max(left.length, right.length);
  const normalizedLeft = left.padEnd(width, "0");
  const normalizedRight = right.padEnd(width, "0");
  return normalizedLeft === normalizedRight
    ? 0
    : normalizedLeft < normalizedRight
      ? -1
      : 1;
}

function compareNumber(left, right) {
  return left === right ? 0 : left < right ? -1 : 1;
}
