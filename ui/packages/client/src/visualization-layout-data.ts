import type { FactoryVisualizationLayoutIssue } from "./visualization-layout-contracts.js";

type InputPath = readonly (string | number)[];
export type InputRecord = Record<string, unknown>;
const MAX_PLAIN_DATA_DEPTH = 64;

interface ValidationFrame {
  value: object;
  path: InputPath;
  depth: number;
  leaving: boolean;
}

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

  const activeContainers = new WeakSet<object>();
  const frames: ValidationFrame[] = [{ value, path, depth: 0, leaving: false }];
  while (frames.length > 0) {
    const frame = frames.pop();
    if (!frame) break;
    if (frame.leaving) {
      activeContainers.delete(frame.value);
      continue;
    }
    if (
      activeContainers.has(frame.value) ||
      frame.depth > MAX_PLAIN_DATA_DEPTH
    ) {
      reportNonPlainData(frame.path, issues);
      continue;
    }

    const isArray = Array.isArray(frame.value);
    let prototype: object | null;
    let keys: readonly PropertyKey[];
    try {
      prototype = Object.getPrototypeOf(frame.value);
      keys = Reflect.ownKeys(frame.value);
    } catch {
      reportNonPlainData(frame.path, issues);
      continue;
    }
    if (
      (isArray && prototype !== Array.prototype) ||
      (!isArray && prototype !== Object.prototype && prototype !== null)
    ) {
      reportNonPlainData(frame.path, issues);
      continue;
    }

    activeContainers.add(frame.value);
    frames.push({ ...frame, leaving: true });
    const childFrames: ValidationFrame[] = [];
    for (const key of keys) {
      if (isArray && key === "length") continue;
      const propertyPath = dataPropertyPath(frame.path, key, isArray);
      let descriptor: PropertyDescriptor | undefined;
      try {
        descriptor = Object.getOwnPropertyDescriptor(frame.value, key);
      } catch {
        reportNonPlainData(propertyPath, issues);
        continue;
      }
      if (
        typeof key !== "string" ||
        descriptor === undefined ||
        !Object.hasOwn(descriptor, "value") ||
        !descriptor.enumerable
      ) {
        reportNonPlainData(propertyPath, issues);
        continue;
      }
      if (isArray && !/^(?:0|[1-9]\d*)$/u.test(key)) {
        reportNonPlainData(propertyPath, issues);
        continue;
      }
      if (typeof descriptor.value === "object" && descriptor.value !== null) {
        childFrames.push({
          value: descriptor.value,
          path: propertyPath,
          depth: frame.depth + 1,
          leaving: false,
        });
      }
    }
    frames.push(...childFrames.reverse());
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
