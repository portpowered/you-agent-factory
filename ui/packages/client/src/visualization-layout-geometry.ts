import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";
import {
  isInvalidCoordinate,
  isInvalidDimension,
  MAX_ANNOTATION_COORDINATE,
  MAX_ANNOTATION_DIMENSION,
  MIN_ANNOTATION_COORDINATE,
} from "./visualization-layout-safety.js";

type InputRecord = Record<string, unknown>;
type InputPath = readonly (string | number)[];

function validateNumericShape(
  input: unknown,
  fields: readonly string[],
  path: InputPath,
  label: string,
  issues: FactoryVisualizationLayoutIssue[],
): InputRecord | undefined {
  if (typeof input !== "object" || input === null || Array.isArray(input)) {
    issues.push({
      category: "structure",
      code: "invalid_type",
      path,
      message: `Expected ${label} to be an object.`,
    });
    return undefined;
  }
  const record = input as InputRecord;
  for (const key of Object.keys(record)) {
    if (!fields.includes(key)) {
      issues.push({
        category: "structure",
        code: "unsupported_field",
        path: [...path, key],
        message: `Unsupported field ${key}.`,
      });
    }
  }
  for (const field of fields) {
    if (!Object.hasOwn(record, field)) {
      issues.push({
        category: "structure",
        code: "missing_required_field",
        path: [...path, field],
        message: `Expected required field ${field}.`,
      });
    } else if (typeof record[field] !== "number") {
      issues.push({
        category: "structure",
        code: "invalid_type",
        path: [...path, field],
        message: `Expected ${field} to be a number.`,
      });
    }
  }
  return record;
}

export function validatePosition(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const position = validateNumericShape(
    input,
    ["x", "y"],
    path,
    "position",
    issues,
  );
  if (!position) return;
  for (const coordinate of ["x", "y"] as const) {
    const value = position[coordinate];
    if (typeof value === "number" && isInvalidCoordinate(value)) {
      issues.push({
        category: "semantic",
        code: "invalid_coordinate",
        path: [...path, coordinate],
        message: `Expected ${coordinate} to be finite and between ${MIN_ANNOTATION_COORDINATE} and ${MAX_ANNOTATION_COORDINATE}.`,
      });
    }
  }
}

export function validateSize(
  input: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  const size = validateNumericShape(
    input,
    ["width", "height"],
    path,
    "size",
    issues,
  );
  if (!size) return;
  for (const dimension of ["width", "height"] as const) {
    const value = size[dimension];
    if (typeof value === "number" && isInvalidDimension(value)) {
      issues.push({
        category: "semantic",
        code: "invalid_dimension",
        path: [...path, dimension],
        message: `Expected ${dimension} to be finite, greater than zero, and at most ${MAX_ANNOTATION_DIMENSION}.`,
      });
    }
  }
}
