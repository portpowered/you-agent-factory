const IDENTITY_KINDS = new Set([
  "completion",
  "dispatch",
  "event",
  "request",
  "session",
  "token",
  "trace",
  "work",
]);

/** Derives one domain-separated identity from stable semantic coordinates. */
export function deriveFactoryEmulatorIdentity(kind, coordinates) {
  if (!IDENTITY_KINDS.has(kind)) {
    throw new TypeError(`unsupported Factory emulator identity kind: ${kind}`);
  }
  const source = `${kind}\u0000${canonicalStringify(coordinates)}`;
  return `emulator-${kind}-${hash64(source, 0xcbf29ce484222325n)}${hash64(
    source,
    0x84222325cbf29cen,
  )}`;
}

/** Produces a key-order-independent representation for deterministic inputs. */
export function canonicalStringify(value) {
  if (value === null || typeof value === "boolean" || typeof value === "string") {
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) {
      throw new TypeError("deterministic identity inputs must contain finite numbers");
    }
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map((entry) => canonicalStringify(entry ?? null)).join(",")}]`;
  }
  if (value && typeof value === "object") {
    const entries = Object.keys(value)
      .filter((key) => value[key] !== undefined)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalStringify(value[key])}`);
    return `{${entries.join(",")}}`;
  }
  throw new TypeError("deterministic identity inputs must contain data values only");
}

function hash64(value, offset) {
  let hash = offset;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= BigInt(value.charCodeAt(index));
    hash = BigInt.asUintN(64, hash * 0x100000001b3n);
  }
  return hash.toString(16).padStart(16, "0");
}
