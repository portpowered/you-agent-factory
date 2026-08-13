import type {
  WorkerSessionEventRecord,
  WorkerTimelineGenericMetadata,
  WorkerTimelineJSONObject,
  WorkerTimelineJSONValue,
} from "./worker-session-timeline-projection-types";

export const MAX_GENERIC_PAYLOAD_KEYS = 16;

export function asObject(value: unknown): WorkerTimelineJSONObject | undefined {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  return value as WorkerTimelineJSONObject;
}

export function optionalString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim().length > 0
    ? value
    : undefined;
}

export function firstString(...values: unknown[]): string | undefined {
  for (const value of values) {
    const result = optionalString(value);
    if (result !== undefined) {
      return result;
    }
  }
  return undefined;
}

export function normalizedToken(value: unknown): string | undefined {
  const result = optionalString(value);
  return result?.toUpperCase();
}

export function finiteNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

export function booleanValue(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

export function copyJSONValue(
  value: unknown,
): WorkerTimelineJSONValue | undefined {
  if (value === null) {
    return null;
  }
  if (typeof value === "string" || typeof value === "boolean") {
    return value;
  }
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : undefined;
  }
  if (Array.isArray(value)) {
    const copy: WorkerTimelineJSONValue[] = [];
    for (const item of value) {
      const itemCopy = copyJSONValue(item);
      if (itemCopy !== undefined) {
        copy.push(itemCopy);
      }
    }
    return copy;
  }
  const object = asObject(value);
  if (object === undefined) {
    return undefined;
  }
  const copy: { [key: string]: WorkerTimelineJSONValue } = {};
  for (const key of Object.keys(object).sort()) {
    const itemCopy = copyJSONValue(object[key]);
    if (itemCopy !== undefined) {
      copy[key] = itemCopy;
    }
  }
  return copy;
}

export function hasKeys(value: object): boolean {
  return Object.keys(value).length > 0;
}

export function genericMetadata(
  record: WorkerSessionEventRecord,
): WorkerTimelineGenericMetadata {
  const payload = asObject(record.payload);
  const payloadKeys = payload === undefined ? [] : Object.keys(payload).sort();
  return {
    schemaId: record.schemaId,
    sourceType: record.sourceType,
    sourceId: record.sourceId,
    sourceSequence: record.sourceSequence,
    payloadKeys: payloadKeys.slice(0, MAX_GENERIC_PAYLOAD_KEYS),
    payloadKeyCount: payloadKeys.length,
    payloadKeysTruncated: payloadKeys.length > MAX_GENERIC_PAYLOAD_KEYS,
  };
}
