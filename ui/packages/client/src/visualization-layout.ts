import type { FactoryDefinition } from "./contracts.js";
import {
  FACTORY_VISUALIZATION_LAYOUT_SCHEMA_VERSION,
  type FactoryVisualizationLayoutIssue,
  type FactoryVisualizationLayoutV1,
  type SafeParseFactoryVisualizationLayoutResult,
} from "./visualization-layout-contracts.js";

export {
  FACTORY_VISUALIZATION_LAYOUT_SCHEMA_VERSION,
  type FactoryVisualizationAnnotation,
  type FactoryVisualizationEmbeddedImageSource,
  type FactoryVisualizationImageAnnotation,
  type FactoryVisualizationImageContent,
  type FactoryVisualizationLayoutIssue,
  type FactoryVisualizationLayoutIssueCode,
  type FactoryVisualizationLayoutV1,
  type FactoryVisualizationNodeEmptyState,
  type FactoryVisualizationNoteAnnotation,
  type FactoryVisualizationNoteTone,
  type FactoryVisualizationPosition,
  type FactoryVisualizationSize,
  type FactoryVisualizationTextEmptyState,
  type SafeParseFactoryVisualizationLayoutResult,
} from "./visualization-layout-contracts.js";

export class FactoryVisualizationLayoutValidationError extends Error {
  readonly issues: readonly FactoryVisualizationLayoutIssue[];

  constructor(issues: readonly FactoryVisualizationLayoutIssue[]) {
    super(
      issues.length === 1
        ? `Factory visualization layout validation failed: ${issues[0]?.message}`
        : `Factory visualization layout validation failed with ${issues.length} issues`,
    );
    this.name = "FactoryVisualizationLayoutValidationError";
    this.issues = issues;
  }
}

type InputRecord = Record<string, unknown>;
type InputPath = readonly (string | number)[];

const layoutFields = new Set([
  "schemaVersion",
  "annotations",
  "nodeEmptyStates",
]);
const noteFields = new Set([
  "id",
  "kind",
  "position",
  "size",
  "title",
  "body",
  "tone",
]);
const imageAnnotationFields = new Set([
  "id",
  "kind",
  "position",
  "size",
  "altText",
  "source",
]);
const positionFields = new Set(["x", "y"]);
const sizeFields = new Set(["width", "height"]);
const emptyStateFields = new Set(["nodeId", "content"]);
const textContentFields = new Set(["kind", "text"]);
const imageContentFields = new Set(["kind", "altText", "source"]);
const imageSourceFields = new Set(["kind", "mediaType", "base64"]);
const imageMediaTypes = new Set(["image/png", "image/jpeg", "image/webp"]);
const noteTones = new Set([
  "neutral",
  "accent",
  "info",
  "success",
  "warning",
  "danger",
]);

function isRecord(value: unknown): value is InputRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function unsupportedFields(
  value: InputRecord,
  fields: ReadonlySet<string>,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  for (const key of Object.keys(value)) {
    if (!fields.has(key)) {
      issues.push({
        category: "structure",
        code: "unsupported_field",
        path: [...path, key],
        message: `Unsupported field ${key}.`,
      });
    }
  }
}

function requiredField(
  value: InputRecord,
  key: string,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): boolean {
  if (key in value) return true;
  issues.push({
    category: "structure",
    code: "missing_required_field",
    path: [...path, key],
    message: `Expected required field ${key}.`,
  });
  return false;
}

function stringField(
  value: InputRecord,
  key: string,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
  required = true,
): boolean {
  if (!requiredField(value, key, path, required ? issues : [])) return false;
  if (!(key in value)) return true;
  if (typeof value[key] === "string") return true;
  issues.push({
    category: "structure",
    code: "invalid_type",
    path: [...path, key],
    message: `Expected ${key} to be a string.`,
  });
  return false;
}

function numberField(
  value: InputRecord,
  key: string,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (!requiredField(value, key, path, issues)) return;
  if (typeof value[key] !== "number") {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path: [...path, key],
      message: `Expected ${key} to be a number.`,
    });
  }
}

function validatePosition(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected position to be an object.",
    });
    return;
  }
  unsupportedFields(input, positionFields, path, issues);
  numberField(input, "x", path, issues);
  numberField(input, "y", path, issues);
}

function validateSize(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected size to be an object.",
    });
    return;
  }
  unsupportedFields(input, sizeFields, path, issues);
  numberField(input, "width", path, issues);
  numberField(input, "height", path, issues);
}

function validateImageSource(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected image source to be an object.",
    });
    return;
  }
  unsupportedFields(input, imageSourceFields, path, issues);
  if (stringField(input, "kind", path, issues) && input.kind !== "embedded") {
    issues.push({
      category: "structure",
      code: "invalid_value",
      path: [...path, "kind"],
      message: "Expected image source kind embedded.",
    });
  }
  if (
    stringField(input, "mediaType", path, issues) &&
    !imageMediaTypes.has(input.mediaType as string)
  ) {
    issues.push({
      category: "structure",
      code: "invalid_value",
      path: [...path, "mediaType"],
      message: "Expected a supported embedded raster media type.",
    });
  }
  stringField(input, "base64", path, issues);
}

function validateImageContent(
  input: InputRecord,
  path: InputPath,
  fields: ReadonlySet<string>,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  unsupportedFields(input, fields, path, issues);
  stringField(input, "altText", path, issues);
  if (requiredField(input, "source", path, issues)) {
    validateImageSource(input.source, [...path, "source"], issues);
  }
}

function validateAnnotation(
  input: unknown,
  index: number,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const path: InputPath = ["annotations", index];
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected annotation to be an object.",
    });
    return;
  }
  stringField(input, "id", path, issues);
  if (!stringField(input, "kind", path, issues)) return;
  if (input.kind !== "note" && input.kind !== "image") {
    issues.push({
      category: "structure",
      code: "invalid_annotation_kind",
      path: [...path, "kind"],
      message: `Unsupported annotation kind ${input.kind}.`,
    });
    return;
  }

  if (input.kind === "note") {
    unsupportedFields(input, noteFields, path, issues);
    stringField(input, "body", path, issues);
    stringField(input, "title", path, issues, false);
    if (
      stringField(input, "tone", path, issues, false) &&
      input.tone !== undefined &&
      !noteTones.has(input.tone as string)
    ) {
      issues.push({
        category: "structure",
        code: "invalid_value",
        path: [...path, "tone"],
        message: `Unsupported note tone ${input.tone}.`,
      });
    }
    if (input.size !== undefined) {
      validateSize(input.size, [...path, "size"], issues);
    }
  } else {
    validateImageContent(input, path, imageAnnotationFields, issues);
    if (requiredField(input, "size", path, issues)) {
      validateSize(input.size, [...path, "size"], issues);
    }
  }

  if (requiredField(input, "position", path, issues)) {
    validatePosition(input.position, [...path, "position"], issues);
  }
}

function validateEmptyStateContent(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected empty-state content to be an object.",
    });
    return;
  }
  if (!stringField(input, "kind", path, issues)) return;
  if (input.kind === "text") {
    unsupportedFields(input, textContentFields, path, issues);
    stringField(input, "text", path, issues);
    return;
  }
  if (input.kind === "image") {
    validateImageContent(input, path, imageContentFields, issues);
    return;
  }
  issues.push({
    category: "structure",
    code: "invalid_empty_state_kind",
    path: [...path, "kind"],
    message: `Unsupported empty-state content kind ${input.kind}.`,
  });
}

function validateNodeEmptyState(
  input: unknown,
  index: number,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const path: InputPath = ["nodeEmptyStates", index];
  if (!isRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected node empty state to be an object.",
    });
    return;
  }
  unsupportedFields(input, emptyStateFields, path, issues);
  stringField(input, "nodeId", path, issues);
  if (requiredField(input, "content", path, issues)) {
    validateEmptyStateContent(input.content, [...path, "content"], issues);
  }
}

function validateDuplicateAnnotationIds(
  annotations: readonly unknown[],
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const indexesById = new Map<string, number[]>();
  for (const [index, annotation] of annotations.entries()) {
    if (!isRecord(annotation) || typeof annotation.id !== "string") continue;
    const indexes = indexesById.get(annotation.id) ?? [];
    indexes.push(index);
    indexesById.set(annotation.id, indexes);
  }
  for (const [id, indexes] of indexesById) {
    if (indexes.length < 2) continue;
    for (const index of indexes) {
      issues.push({
        category: "semantic",
        code: "duplicate_annotation_id",
        path: ["annotations", index, "id"],
        message: `Annotation ID ${id} is duplicated at annotation indexes ${indexes.join(", ")}.`,
      });
    }
  }
}

/**
 * Validate presentation metadata beside an immutable canonical Factory.
 * The Factory is accepted as context but is never modified or included in the result.
 */
export function safeParseFactoryVisualizationLayout(
  input: unknown,
  _factory: Readonly<FactoryDefinition>,
): SafeParseFactoryVisualizationLayoutResult {
  if (!isRecord(input)) {
    return {
      success: false,
      issues: [
        {
          category: "structure",
          code: "invalid_type",
          path: [],
          message: "Expected Factory visualization layout to be an object.",
        },
      ],
    };
  }

  const issues: FactoryVisualizationLayoutIssue[] = [];
  unsupportedFields(input, layoutFields, [], issues);
  if (
    stringField(input, "schemaVersion", [], issues) &&
    input.schemaVersion !== FACTORY_VISUALIZATION_LAYOUT_SCHEMA_VERSION
  ) {
    issues.push({
      category: "structure",
      code: "unsupported_layout_schema_version",
      path: ["schemaVersion"],
      message: `Unsupported Factory visualization layout schema version: ${input.schemaVersion}.`,
    });
  }

  if (input.annotations !== undefined) {
    if (!Array.isArray(input.annotations)) {
      issues.push({
        category: "structure",
        code: "invalid_type",
        path: ["annotations"],
        message: "Expected annotations to be an array.",
      });
    } else {
      for (const [index, annotation] of input.annotations.entries()) {
        validateAnnotation(annotation, index, issues);
      }
      validateDuplicateAnnotationIds(input.annotations, issues);
    }
  }

  if (input.nodeEmptyStates !== undefined) {
    if (!Array.isArray(input.nodeEmptyStates)) {
      issues.push({
        category: "structure",
        code: "invalid_type",
        path: ["nodeEmptyStates"],
        message: "Expected nodeEmptyStates to be an array.",
      });
    } else {
      for (const [index, emptyState] of input.nodeEmptyStates.entries()) {
        validateNodeEmptyState(emptyState, index, issues);
      }
    }
  }

  return issues.length > 0
    ? { success: false, issues }
    : {
        success: true,
        data: input as unknown as FactoryVisualizationLayoutV1,
      };
}

export function parseFactoryVisualizationLayout(
  input: unknown,
  factory: Readonly<FactoryDefinition>,
): FactoryVisualizationLayoutV1 {
  const result = safeParseFactoryVisualizationLayout(input, factory);
  if (!result.success) {
    throw new FactoryVisualizationLayoutValidationError(result.issues);
  }
  return result.data;
}
