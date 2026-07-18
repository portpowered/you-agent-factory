import Ajv2020, {
  type ErrorObject,
  type ValidateFunction,
} from "ajv/dist/2020.js";
import addFormats from "ajv-formats";

import factorySchema from "./generated/factory.schema.json" with {
  type: "json",
};
import factoryEventSchema from "./generated/factory-event.schema.json" with {
  type: "json",
};

type InputRecord = Record<string, unknown>;
type JsonSchema = InputRecord & {
  oneOf?: readonly InputRecord[];
  properties?: InputRecord;
};

export type CanonicalSchemaIssueCode =
  | "invalid_type"
  | "invalid_value"
  | "missing_required_field"
  | "unsupported_event_type"
  | "unsupported_field";

export interface CanonicalSchemaIssue {
  category: "structure";
  code: CanonicalSchemaIssueCode;
  path: readonly (string | number)[];
  message: string;
}

const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
addFormats(ajv);

const canonicalEventSchema = factoryEventSchema as unknown as JsonSchema;
const {
  $id: _eventSchemaId,
  discriminator: _eventDiscriminator,
  oneOf: eventVariants = [],
  ...eventEnvelopeSchema
} = canonicalEventSchema;
const validateEventEnvelopeShape = ajv.compile(eventEnvelopeSchema);
const validateFactoryShape = ajv.compile(
  factorySchema as unknown as JsonSchema,
);
const eventValidators = new Map<string, ValidateFunction>();

function isRecord(value: unknown): value is InputRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

for (const variant of eventVariants) {
  const variantProperties = variant.properties;
  const typeSchemaInput = isRecord(variantProperties)
    ? variantProperties.type
    : undefined;
  const typeSchema = isRecord(typeSchemaInput) ? typeSchemaInput : undefined;
  const eventType = typeSchema?.const;
  if (typeof eventType !== "string") {
    continue;
  }
  eventValidators.set(
    eventType,
    ajv.compile({
      ...eventEnvelopeSchema,
      properties: {
        ...eventEnvelopeSchema.properties,
        ...(isRecord(variantProperties) ? variantProperties : {}),
      },
    }),
  );
}

function pointerSegments(pointer: string): string[] {
  if (pointer.length === 0) {
    return [];
  }
  return pointer
    .slice(1)
    .split("/")
    .map((segment) => segment.replaceAll("~1", "/").replaceAll("~0", "~"));
}

function issuePath(
  error: ErrorObject,
  path: readonly (string | number)[],
): readonly (string | number)[] {
  const segments: (string | number)[] = pointerSegments(error.instancePath).map(
    (segment) => (/^(0|[1-9]\d*)$/.test(segment) ? Number(segment) : segment),
  );
  const property =
    error.keyword === "required"
      ? error.params.missingProperty
      : error.keyword === "additionalProperties"
        ? error.params.additionalProperty
        : undefined;
  return typeof property === "string"
    ? [...path, ...segments, property]
    : [...path, ...segments];
}

function issueCode(error: ErrorObject): CanonicalSchemaIssueCode {
  if (error.keyword === "required") return "missing_required_field";
  if (error.keyword === "additionalProperties") return "unsupported_field";
  if (error.keyword === "type") return "invalid_type";
  if (error.keyword === "enum" && error.instancePath === "/type") {
    return "unsupported_event_type";
  }
  return "invalid_value";
}

function issues(
  errors: readonly ErrorObject[] | null | undefined,
  path: readonly (string | number)[],
): CanonicalSchemaIssue[] {
  return (errors ?? [])
    .filter(
      (error) =>
        !(error.keyword === "enum" && error.instancePath === "/schemaVersion"),
    )
    .map((error) => {
      const pathToIssue = issuePath(error, path);
      const field = pathToIssue.at(-1);
      return {
        category: "structure",
        code: issueCode(error),
        path: pathToIssue,
        message:
          error.keyword === "required"
            ? `Expected required field ${String(field)}.`
            : error.keyword === "additionalProperties"
              ? `Unsupported field ${String(field)}.`
              : `Expected ${pathToIssue.join(".") || "value"} to satisfy the canonical contract: ${error.message ?? error.keyword}.`,
      };
    });
}

export function canonicalFactoryIssues(
  input: unknown,
  path: readonly (string | number)[],
): CanonicalSchemaIssue[] {
  return validateFactoryShape(input)
    ? []
    : issues(validateFactoryShape.errors, path);
}

export function canonicalEventIssues(
  input: unknown,
  path: readonly (string | number)[],
): CanonicalSchemaIssue[] {
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
  const validator =
    typeof input.type === "string"
      ? (eventValidators.get(input.type) ?? validateEventEnvelopeShape)
      : validateEventEnvelopeShape;
  return validator(input) ? [] : issues(validator.errors, path);
}
