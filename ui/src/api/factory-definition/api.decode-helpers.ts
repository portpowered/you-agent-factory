import type { components } from "../generated/openapi";

type FactorySchemas = components["schemas"];
type FactoryVersion = FactorySchemas["HybridLogicalTimestamp"];

export class FactoryDefinitionAPIError extends Error {
  public constructor(message: string) {
    super(message);
    this.name = "FactoryDefinitionAPIError";
  }
}

export function expectObject(
  value: unknown,
  path: string,
): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new FactoryDefinitionAPIError(`${path} must be an object.`);
  }
  return { ...value };
}

export function rejectUnknownKeys(
  value: Record<string, unknown>,
  allowedKeys: Set<string>,
  path: string,
): void {
  for (const key of Object.keys(value)) {
    if (allowedKeys.has(key)) {
      continue;
    }
    throw new FactoryDefinitionAPIError(
      `${path}.${key} is not allowed by the generated factory contract.`,
    );
  }
}

export function readOptionalString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (typeof item !== "string") {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be a string.`);
  }
  return item;
}

export function readRequiredString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string {
  const item = readOptionalString(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

export function readOptionalBoolean(
  value: Record<string, unknown>,
  key: string,
  path: string,
): boolean | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (typeof item !== "boolean") {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be a boolean.`);
  }
  return item;
}

export function readOptionalInteger(
  value: Record<string, unknown>,
  key: string,
  path: string,
): number | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (typeof item !== "number" || !Number.isInteger(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an integer.`);
  }
  return item;
}

export function readRequiredInteger(
  value: Record<string, unknown>,
  key: string,
  path: string,
): number {
  const item = readOptionalInteger(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

export function readRequiredNumber(
  value: Record<string, unknown>,
  key: string,
  path: string,
): number {
  const item = value[key];
  if (item === undefined || item === null) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  if (typeof item !== "number" || !Number.isFinite(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be a number.`);
  }
  return item;
}

export function readOptionalEnum<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T | undefined {
  const item = readOptionalString(value, key, path);
  if (item === undefined) {
    return undefined;
  }
  if (!allowedValues.has(item as T)) {
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be one of ${Array.from(allowedValues).join(", ")}.`,
    );
  }
  return item as T;
}

export function readRequiredEnum<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T {
  const item = readOptionalEnum(value, key, path, allowedValues);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

export function readOptionalEnumArray<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T[] | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (!Array.isArray(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an array.`);
  }
  return item.map((entry, index) => {
    const entryPath = `${path}.${key}[${index}]`;
    if (typeof entry !== "string") {
      throw new FactoryDefinitionAPIError(`${entryPath} must be a string.`);
    }
    if (!allowedValues.has(entry as T)) {
      throw new FactoryDefinitionAPIError(
        `${entryPath} must be one of ${Array.from(allowedValues).join(", ")}.`,
      );
    }
    return entry as T;
  });
}

export function readRequiredEnumArray<T extends string>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  allowedValues: Set<T>,
): T[] {
  const item = readOptionalEnumArray(value, key, path, allowedValues);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

export function readOptionalStringArray(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string[] | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (!Array.isArray(item)) {
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be an array of strings.`,
    );
  }
  return item.map((entry, index) => {
    if (typeof entry !== "string") {
      throw new FactoryDefinitionAPIError(
        `${path}.${key}[${index}] must be a string.`,
      );
    }
    return entry;
  });
}

export function readRequiredStringArray(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string[] {
  const item = readOptionalStringArray(value, key, path);
  if (item === undefined) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return item;
}

export function readOptionalStringMap(
  value: Record<string, unknown>,
  key: string,
  path: string,
): Record<string, string> | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }

  const record = expectObject(item, `${path}.${key}`);
  const stringMap: Record<string, string> = {};
  for (const [mapKey, mapValue] of Object.entries(record)) {
    if (typeof mapValue !== "string") {
      throw new FactoryDefinitionAPIError(
        `${path}.${key}.${mapKey} must be a string.`,
      );
    }
    stringMap[mapKey] = mapValue;
  }
  return stringMap;
}

export function readOptionalNullableString(
  value: Record<string, unknown>,
  key: string,
  path: string,
): string | null | undefined {
  const item = value[key];
  if (item === undefined) {
    return undefined;
  }
  if (item === null) {
    return null;
  }
  if (typeof item !== "string") {
    throw new FactoryDefinitionAPIError(
      `${path}.${key} must be a string or null.`,
    );
  }
  return item;
}

export function readOptionalObject<T>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  decode: (input: unknown, valuePath: string) => T,
): T | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  return decode(item, `${path}.${key}`);
}

export function readOptionalArray<T>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  decode: (input: unknown, valuePath: string) => T,
): T[] | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  if (!Array.isArray(item)) {
    throw new FactoryDefinitionAPIError(`${path}.${key} must be an array.`);
  }
  return item.map((entry, index) => decode(entry, `${path}.${key}[${index}]`));
}

export function readRequiredArray<T>(
  value: Record<string, unknown>,
  key: string,
  path: string,
  decode: (input: unknown, valuePath: string) => T,
): T[] {
  if (value[key] === undefined || value[key] === null) {
    throw new FactoryDefinitionAPIError(`${path}.${key} is required.`);
  }
  return readOptionalArray(value, key, path, decode) as T[];
}

export function readOptionalFactoryVersion(
  value: Record<string, unknown>,
  key: string,
  path: string,
): FactoryVersion | undefined {
  const item = value[key];
  if (item === undefined || item === null) {
    return undefined;
  }
  const record = expectObject(item, `${path}.${key}`);
  const logical = record.logical;
  const physical = record.physical;
  if (!isFactoryVersionLogicalValue(logical)) {
    throw new FactoryDefinitionAPIError(
      `${path}.${key}.logical must be a decimal string.`,
    );
  }
  if (typeof physical !== "string") {
    throw new FactoryDefinitionAPIError(
      `${path}.${key}.physical must be a string.`,
    );
  }
  return {
    logical: String(logical),
    physical,
  };
}

function isFactoryVersionLogicalValue(
  value: unknown,
): value is number | string {
  if (typeof value === "string") {
    return /^[0-9]+$/.test(value);
  }
  return typeof value === "number" && Number.isFinite(value);
}
