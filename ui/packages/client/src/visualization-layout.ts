import type { FactoryDefinition } from "./contracts.js";
import {
  FACTORY_VISUALIZATION_LAYOUT_SCHEMA_VERSION,
  type FactoryVisualizationLayoutIssue,
  type FactoryVisualizationLayoutV1,
  type SafeParseFactoryVisualizationLayoutResult,
} from "./visualization-layout-contracts.js";
import {
  clonePlainData,
  type InputRecord,
  isInputRecord,
  validatePlainDataContainers,
} from "./visualization-layout-data.js";
import { FactoryVisualizationLayoutValidationError } from "./visualization-layout-error.js";
import {
  emptyStateFields,
  imageAnnotationFields,
  imageContentFields,
  imageSourceFields,
  layoutFields,
  noteFields,
  textContentFields,
} from "./visualization-layout-fields.js";
import {
  validatePosition,
  validateSize,
} from "./visualization-layout-geometry.js";
import {
  type ImageByteBudget,
  MAX_IMAGE_ALT_TEXT_LENGTH,
  validateEmbeddedImageData,
} from "./visualization-layout-media.js";
import {
  isUnsafeContentField,
  MAX_EMPTY_STATE_TEXT_LENGTH,
  MAX_NOTE_BODY_LENGTH,
  MAX_NOTE_TITLE_LENGTH,
  validateDuplicateAnnotationIds,
  validateDuplicateNodeIds,
  validatePlainText,
} from "./visualization-layout-safety.js";
import {
  factoryVisualizationCanonicalNodeIds,
  validateCanonicalNodeId,
} from "./visualization-layout-topology.js";

type InputPath = readonly (string | number)[];
const imageMediaTypes = new Set(["image/png", "image/jpeg", "image/webp"]);
const noteTones = new Set([
  "neutral",
  "accent",
  "info",
  "success",
  "warning",
  "danger",
]);
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
      if (isUnsafeContentField(key)) {
        issues.push({
          category: "semantic",
          code: "unsafe_content_field",
          path: [...path, key],
          message: `Executable, connection-like, and URI-bearing content field ${key} is not allowed.`,
        });
      }
    }
  }
}
function requiredField(
  value: InputRecord,
  key: string,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): boolean {
  if (Object.hasOwn(value, key)) return true;
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
  if (!Object.hasOwn(value, key)) return true;
  if (typeof value[key] === "string") return true;
  issues.push({
    category: "structure",
    code: "invalid_type",
    path: [...path, key],
    message: `Expected ${key} to be a string.`,
  });
  return false;
}

function validateImageSource(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
  imageBudget: ImageByteBudget,
): void {
  if (!isInputRecord(input)) {
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
      category: "semantic",
      code: "unsupported_image_media_type",
      path: [...path, "mediaType"],
      message: "Expected a supported embedded raster media type.",
    });
  }
  const validBase64Type = stringField(input, "base64", path, issues);
  if (
    validBase64Type &&
    typeof input.base64 === "string" &&
    typeof input.mediaType === "string" &&
    imageMediaTypes.has(input.mediaType)
  ) {
    validateEmbeddedImageData(
      input.base64,
      input.mediaType,
      [...path, "base64"],
      issues,
      imageBudget,
    );
  }
}

function validateImageContent(
  input: InputRecord,
  path: InputPath,
  fields: ReadonlySet<string>,
  issues: FactoryVisualizationLayoutIssue[],
  imageBudget: ImageByteBudget,
): void {
  unsupportedFields(input, fields, path, issues);
  if (stringField(input, "altText", path, issues)) {
    validatePlainText(
      input.altText as string,
      [...path, "altText"],
      MAX_IMAGE_ALT_TEXT_LENGTH,
      "image alternative text",
      issues,
      true,
    );
  }
  if (requiredField(input, "source", path, issues)) {
    validateImageSource(input.source, [...path, "source"], issues, imageBudget);
  }
}

function validateAnnotation(
  input: unknown,
  index: number,
  issues: FactoryVisualizationLayoutIssue[],
  imageBudget: ImageByteBudget,
): void {
  const path: InputPath = ["annotations", index];
  if (!isInputRecord(input)) {
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
    if (stringField(input, "body", path, issues)) {
      validatePlainText(
        input.body as string,
        [...path, "body"],
        MAX_NOTE_BODY_LENGTH,
        "note body",
        issues,
        true,
      );
    }
    if (
      stringField(input, "title", path, issues, false) &&
      typeof input.title === "string"
    ) {
      validatePlainText(
        input.title,
        [...path, "title"],
        MAX_NOTE_TITLE_LENGTH,
        "note title",
        issues,
        false,
      );
    }
    if (
      stringField(input, "tone", path, issues, false) &&
      input.tone !== undefined &&
      !noteTones.has(input.tone as string)
    ) {
      issues.push({
        category: "structure",
        code: "unsupported_note_tone",
        path: [...path, "tone"],
        message: `Unsupported note tone ${input.tone}.`,
      });
    }
    if (input.size !== undefined) {
      validateSize(input.size, [...path, "size"], issues);
    }
  } else {
    validateImageContent(
      input,
      path,
      imageAnnotationFields,
      issues,
      imageBudget,
    );
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
  imageBudget: ImageByteBudget,
): void {
  if (!isInputRecord(input)) {
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
    if (stringField(input, "text", path, issues)) {
      validatePlainText(
        input.text as string,
        [...path, "text"],
        MAX_EMPTY_STATE_TEXT_LENGTH,
        "node empty-state text",
        issues,
        true,
      );
    }
    return;
  }
  if (input.kind === "image") {
    validateImageContent(input, path, imageContentFields, issues, imageBudget);
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
  imageBudget: ImageByteBudget,
  canonicalNodeIds: ReadonlySet<string>,
): void {
  const path: InputPath = ["nodeEmptyStates", index];
  if (!isInputRecord(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: "Expected node empty state to be an object.",
    });
    return;
  }
  unsupportedFields(input, emptyStateFields, path, issues);
  if (stringField(input, "nodeId", path, issues)) {
    validateCanonicalNodeId(
      input.nodeId as string,
      [...path, "nodeId"],
      canonicalNodeIds,
      issues,
    );
  }
  if (requiredField(input, "content", path, issues)) {
    validateEmptyStateContent(
      input.content,
      [...path, "content"],
      issues,
      imageBudget,
    );
  }
}

/**
 * Validate presentation metadata beside an immutable canonical Factory.
 * The Factory is accepted as context but is never modified or included in the result.
 */
export function safeParseFactoryVisualizationLayout(
  input: unknown,
  factory: Readonly<FactoryDefinition>,
): SafeParseFactoryVisualizationLayoutResult {
  if (!isInputRecord(input)) {
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
  validatePlainDataContainers(input, [], issues);
  if (issues.length > 0) return { success: false, issues };
  const imageBudget: ImageByteBudget = {
    total: 0,
    aggregateLimitReported: false,
  };
  const canonicalNodeIds = factoryVisualizationCanonicalNodeIds(factory);
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
        validateAnnotation(annotation, index, issues, imageBudget);
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
        validateNodeEmptyState(
          emptyState,
          index,
          issues,
          imageBudget,
          canonicalNodeIds,
        );
      }
      validateDuplicateNodeIds(input.nodeEmptyStates, issues);
    }
  }

  return issues.length > 0
    ? { success: false, issues }
    : {
        success: true,
        data: clonePlainData(input) as FactoryVisualizationLayoutV1,
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
