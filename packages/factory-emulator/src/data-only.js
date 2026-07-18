const DATA_ONLY_EXPECTATION =
  "plain objects, dense arrays, null, booleans, strings, or finite numbers";

/** Returns deterministic JSON-Pointer diagnostics for values outside the data-only graph. */
export function dataOnlyDiagnostics(value, { code, rootPath = "/" }) {
  const diagnostics = [];
  visit(value, rootPath, code, new Set(), diagnostics);
  return diagnostics;
}

function visit(value, path, code, ancestors, diagnostics) {
  if (
    value === null ||
    typeof value === "string" ||
    typeof value === "boolean" ||
    (typeof value === "number" && Number.isFinite(value))
  ) {
    return;
  }
  if (typeof value !== "object") {
    diagnostics.push(diagnostic(code, path, typeof value));
    return;
  }

  let prototype;
  let descriptors;
  try {
    prototype = Object.getPrototypeOf(value);
    descriptors = Object.getOwnPropertyDescriptors(value);
  } catch {
    diagnostics.push(diagnostic(code, path, "an uninspectable object"));
    return;
  }
  if (prototype !== Object.prototype && prototype !== Array.prototype) {
    diagnostics.push(diagnostic(code, path, prototype?.constructor?.name ?? "an object"));
    return;
  }
  if (ancestors.has(value)) {
    diagnostics.push(diagnostic(code, path, "a circular reference"));
    return;
  }

  ancestors.add(value);
  if (Array.isArray(value)) {
    visitArray(value, path, code, ancestors, diagnostics, descriptors);
  } else {
    visitRecord(path, code, ancestors, diagnostics, descriptors);
  }
  ancestors.delete(value);
}

function visitArray(value, path, code, ancestors, diagnostics, descriptors) {
  for (let index = 0; index < value.length; index += 1) {
    const childPath = joinPath(path, String(index));
    const descriptor = descriptors[index];
    if (descriptor === undefined) {
      diagnostics.push(diagnostic(code, childPath, "a sparse array slot"));
      continue;
    }
    visitDescriptor(descriptor, childPath, code, ancestors, diagnostics);
  }
  for (const key of Reflect.ownKeys(descriptors)) {
    if (key === "length" || (typeof key === "string" && isArrayIndex(key, value.length))) {
      continue;
    }
    diagnostics.push(diagnostic(code, joinPath(path, String(key)), "an array property"));
  }
}

function visitRecord(path, code, ancestors, diagnostics, descriptors) {
  for (const key of Reflect.ownKeys(descriptors)) {
    const childPath = typeof key === "symbol" ? path : joinPath(path, key);
    if (typeof key === "symbol") {
      diagnostics.push(diagnostic(code, childPath, "a symbol-keyed property"));
      continue;
    }
    visitDescriptor(descriptors[key], childPath, code, ancestors, diagnostics);
  }
}

function visitDescriptor(descriptor, path, code, ancestors, diagnostics) {
  if (!descriptor.enumerable || !("value" in descriptor)) {
    diagnostics.push(diagnostic(code, path, "an accessor or non-enumerable property"));
    return;
  }
  visit(descriptor.value, path, code, ancestors, diagnostics);
}

function isArrayIndex(key, length) {
  const index = Number(key);
  return Number.isInteger(index) && index >= 0 && index < length && String(index) === key;
}

function joinPath(parent, property) {
  const escaped = property.replaceAll("~", "~0").replaceAll("/", "~1");
  return parent === "/" ? `/${escaped}` : `${parent}/${escaped}`;
}

function diagnostic(code, path, received) {
  return {
    code,
    path,
    message: `${path} must contain data-only values; received ${received}.`,
    expectation: DATA_ONLY_EXPECTATION,
  };
}
