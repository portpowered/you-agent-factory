import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";

type InputPath = readonly (string | number)[];
export type InputRecord = Record<string, unknown>;

export function isInputRecord(value: unknown): value is InputRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function dataPropertyPath(
  path: InputPath,
  key: PropertyKey,
  isArray: boolean,
): InputPath {
  if (typeof key !== "string") return path;
  return [
    ...path,
    isArray && /^(?:0|[1-9]\d*)$/u.test(key) ? Number(key) : key,
  ];
}

function reportNonPlainData(
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  issues.push({
    category: "structure",
    code: "non_plain_data",
    path,
    message:
      "Expected plain data with standard containers and own data properties.",
  });
}

/** Reject behavior-bearing containers before the parser reads any input value. */
export function validatePlainDataContainers(
  value: unknown,
  path: InputPath,
  issues: FactoryVisualizationLayoutIssue[],
): void {
  if (typeof value !== "object" || value === null) return;

  const isArray = Array.isArray(value);
  const prototype = Object.getPrototypeOf(value);
  if (
    (isArray && prototype !== Array.prototype) ||
    (!isArray && prototype !== Object.prototype && prototype !== null)
  ) {
    reportNonPlainData(path, issues);
    return;
  }

  for (const key of Reflect.ownKeys(value)) {
    if (isArray && key === "length") continue;
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    const propertyPath = dataPropertyPath(path, key, isArray);
    if (
      typeof key !== "string" ||
      descriptor === undefined ||
      !("value" in descriptor) ||
      !descriptor.enumerable
    ) {
      reportNonPlainData(propertyPath, issues);
      continue;
    }
    if (isArray && !/^(?:0|[1-9]\d*)$/u.test(key)) {
      reportNonPlainData(propertyPath, issues);
      continue;
    }
    validatePlainDataContainers(descriptor.value, propertyPath, issues);
  }
}

/** Copy validated values into fresh standard containers without prototype data. */
export function clonePlainData(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(clonePlainData);
  if (typeof value !== "object" || value === null) return value;

  const clone: Record<string, unknown> = {};
  for (const [key, child] of Object.entries(value)) {
    clone[key] = clonePlainData(child);
  }
  return clone;
}
