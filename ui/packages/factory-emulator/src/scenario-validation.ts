import Ajv2020, { type ErrorObject } from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { generatedScenarioSchema } from "./generated/scenario-schema.js";
import {
  type FactoryEmulatorScenarioIssue,
  type FactoryEmulatorScenarioIssueCode,
  isFactoryEmulatorScenarioCompatibilityIssueCode,
} from "./scenario-contracts.js";

const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
addFormats(ajv);
const validateScenarioShape = ajv.compile(generatedScenarioSchema);

const tolerantAjv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: "all",
  strict: false,
  useDefaults: false,
});
addFormats(tolerantAjv);
const validateScenarioShapeWithoutAdditional = tolerantAjv.compile(
  generatedScenarioSchema,
);

function pointerSegments(pointer: string): (string | number)[] {
  if (pointer.length === 0) return [];
  return pointer
    .slice(1)
    .split("/")
    .map((segment) => segment.replaceAll("~1", "/").replaceAll("~0", "~"))
    .map((segment) =>
      /^(0|[1-9]\d*)$/.test(segment) ? Number(segment) : segment,
    );
}

function errorPath(error: ErrorObject): readonly (string | number)[] {
  const path = pointerSegments(error.instancePath);
  if (error.keyword === "required") {
    return [...path, error.params.missingProperty];
  }
  if (error.keyword === "additionalProperties") {
    return [...path, error.params.additionalProperty];
  }
  return path;
}

function structuralIssueCode(
  error: ErrorObject,
  path: readonly (string | number)[],
): FactoryEmulatorScenarioIssueCode {
  if (error.keyword === "required") return "missing_required_field";
  if (error.keyword === "additionalProperties") return "unsupported_field";
  if (error.keyword === "type") return "invalid_type";
  if (path.join(".") === "schemaVersion") return "unsupported_schema_version";
  if (path.join(".") === "startAt") return "invalid_start_at";
  if (
    error.keyword === "pattern" &&
    ["id", "name"].includes(String(path.at(-1)))
  ) {
    return "unstable_identity";
  }
  if (path.includes("cursor")) return "invalid_cursor";
  if (path.at(-1) === "exhaustion") return "invalid_exhaustion";
  if (path.includes("unmatched")) return "invalid_unmatched";
  if (path.includes("outcomes") || path.at(-1) === "outcome") {
    return "invalid_outcome";
  }
  return "invalid_value";
}

function structuralIssues(
  errors: readonly ErrorObject[] | null | undefined,
): FactoryEmulatorScenarioIssue[] {
  const results: FactoryEmulatorScenarioIssue[] = [];
  const seen = new Set<string>();
  for (const error of errors ?? []) {
    if (error.keyword === "oneOf") continue;
    const path = errorPath(error);
    const code = structuralIssueCode(error, path);
    const field = path.at(-1);
    const message =
      error.keyword === "required"
        ? `Expected required field ${String(field)}.`
        : error.keyword === "additionalProperties"
          ? `Unsupported field ${String(field)}.`
          : `Expected ${path.join(".") || "scenario"} to satisfy the scenario contract: ${error.message ?? error.keyword}.`;
    const key = `${code}:${path.join(".")}:${message}`;
    if (!seen.has(key)) {
      seen.add(key);
      results.push({ category: "structure", code, path, message });
    }
  }
  return results;
}

function partitionValidationIssues(
  issues: readonly FactoryEmulatorScenarioIssue[],
): {
  readonly blockingIssues: FactoryEmulatorScenarioIssue[];
  readonly diagnostics: FactoryEmulatorScenarioIssue[];
} {
  const blockingIssues: FactoryEmulatorScenarioIssue[] = [];
  const diagnostics: FactoryEmulatorScenarioIssue[] = [];
  for (const issue of issues) {
    if (isFactoryEmulatorScenarioCompatibilityIssueCode(issue.code)) {
      diagnostics.push(issue);
    } else {
      blockingIssues.push(issue);
    }
  }
  return { blockingIssues, diagnostics };
}

function cloneForStructuralValidation(input: unknown): unknown {
  try {
    return JSON.parse(JSON.stringify(input)) as unknown;
  } catch {
    return undefined;
  }
}

export interface FactoryEmulatorScenarioStructuralValidation {
  readonly blockingIssues: readonly FactoryEmulatorScenarioIssue[];
  readonly diagnostics: readonly FactoryEmulatorScenarioIssue[];
}

export function validateFactoryEmulatorScenarioStructure(
  input: unknown,
): FactoryEmulatorScenarioStructuralValidation {
  const shapeIsValid = validateScenarioShape(input);
  const structural = shapeIsValid
    ? []
    : structuralIssues(validateScenarioShape.errors);
  let { blockingIssues, diagnostics } = partitionValidationIssues(structural);
  if (diagnostics.length > 0) {
    const validationInput = cloneForStructuralValidation(input);
    const tolerantShapeIsValid =
      validateScenarioShapeWithoutAdditional(validationInput);
    blockingIssues = tolerantShapeIsValid
      ? []
      : partitionValidationIssues(
          structuralIssues(validateScenarioShapeWithoutAdditional.errors),
        ).blockingIssues;
  }
  return { blockingIssues, diagnostics };
}
