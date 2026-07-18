const DATA_ONLY_EXPECTATION =
  "plain objects, dense arrays, null, booleans, strings, or finite numbers";

const MAXIMUM_DATA_ONLY_NODES = 10_000;
const MAXIMUM_DATA_ONLY_DEPTH = 100;

/** Returns deterministic JSON-Pointer diagnostics for values outside the data-only graph. */
export function dataOnlyDiagnostics(value, { code, rootPath = "/" }) {
  const diagnostics = [];
  const ancestors = new Set();
  const stack = [{ depth: 0, kind: "visit", path: rootPath, value }];
  let inspectedNodes = 0;

  while (stack.length > 0) {
    const frame = stack.pop();
    if (frame.kind === "leave") {
      ancestors.delete(frame.value);
      continue;
    }

    inspectedNodes += 1;
    if (inspectedNodes > MAXIMUM_DATA_ONLY_NODES) {
      diagnostics.push(boundDiagnostic(
        code,
        frame.path,
        `more than ${MAXIMUM_DATA_ONLY_NODES} data nodes`,
      ));
      break;
    }
    if (frame.depth > MAXIMUM_DATA_ONLY_DEPTH) {
      diagnostics.push(boundDiagnostic(
        code,
        frame.path,
        `data nested deeper than ${MAXIMUM_DATA_ONLY_DEPTH} levels`,
      ));
      break;
    }
    if (isPrimitive(frame.value)) {
      continue;
    }
    if (typeof frame.value !== "object") {
      diagnostics.push(diagnostic(code, frame.path, typeof frame.value));
      continue;
    }

    const inspection = inspectObject(frame.value, frame.path, code, ancestors);
    if (inspection.diagnostic !== undefined) {
      diagnostics.push(inspection.diagnostic);
      continue;
    }
    if (inspection.children.length > MAXIMUM_DATA_ONLY_NODES - inspectedNodes) {
      diagnostics.push(boundDiagnostic(
        code,
        frame.path,
        `more than ${MAXIMUM_DATA_ONLY_NODES} data nodes`,
      ));
      break;
    }

    ancestors.add(frame.value);
    stack.push({ kind: "leave", value: frame.value });
    for (let index = inspection.children.length - 1; index >= 0; index -= 1) {
      stack.push({
        ...inspection.children[index],
        depth: frame.depth + 1,
        kind: "visit",
      });
    }
    diagnostics.push(...inspection.propertyDiagnostics);
  }
  return diagnostics;
}

function isPrimitive(value) {
  return (
    value === null ||
    typeof value === "string" ||
    typeof value === "boolean" ||
    (typeof value === "number" && Number.isFinite(value))
  );
}

function inspectObject(value, path, code, ancestors) {
  let prototype;
  let descriptors;
  try {
    prototype = Object.getPrototypeOf(value);
    if (prototype !== Object.prototype && prototype !== Array.prototype) {
      return { diagnostic: diagnostic(code, path, "an object with an unsupported prototype") };
    }
    if (ancestors.has(value)) {
      return { diagnostic: diagnostic(code, path, "a circular reference") };
    }
    if (Array.isArray(value) && value.length > MAXIMUM_DATA_ONLY_NODES) {
      return {
        diagnostic: boundDiagnostic(
          code,
          path,
          `an array longer than ${MAXIMUM_DATA_ONLY_NODES} entries`,
        ),
      };
    }
    if (!Array.isArray(value) && hasTooManyEnumerableProperties(value)) {
      return {
        diagnostic: boundDiagnostic(
          code,
          path,
          `an object with more than ${MAXIMUM_DATA_ONLY_NODES} entries`,
        ),
      };
    }
    descriptors = Object.getOwnPropertyDescriptors(value);
  } catch {
    return { diagnostic: diagnostic(code, path, "an uninspectable object") };
  }

  return Array.isArray(value)
    ? inspectArray(value, path, code, descriptors)
    : inspectRecord(path, code, descriptors);
}

function hasTooManyEnumerableProperties(value) {
  let count = 0;
  for (const key in value) {
    if (!Object.hasOwn(value, key)) {
      continue;
    }
    count += 1;
    if (count > MAXIMUM_DATA_ONLY_NODES) {
      return true;
    }
  }
  return false;
}

function inspectArray(value, path, code, descriptors) {
  const children = [];
  const propertyDiagnostics = [];
  for (let index = 0; index < value.length; index += 1) {
    const childPath = joinPath(path, String(index));
    const descriptor = descriptors[index];
    if (descriptor === undefined) {
      propertyDiagnostics.push(diagnostic(code, childPath, "a sparse array slot"));
      continue;
    }
    inspectDescriptor(descriptor, childPath, code, children, propertyDiagnostics);
  }
  for (const key of Reflect.ownKeys(descriptors)) {
    if (key === "length" || (typeof key === "string" && isArrayIndex(key, value.length))) {
      continue;
    }
    propertyDiagnostics.push(
      diagnostic(code, joinPath(path, String(key)), "an array property"),
    );
  }
  return { children, propertyDiagnostics };
}

function inspectRecord(path, code, descriptors) {
  const children = [];
  const propertyDiagnostics = [];
  for (const key of Reflect.ownKeys(descriptors)) {
    const childPath = typeof key === "symbol" ? path : joinPath(path, key);
    if (typeof key === "symbol") {
      propertyDiagnostics.push(diagnostic(code, childPath, "a symbol-keyed property"));
      continue;
    }
    inspectDescriptor(descriptors[key], childPath, code, children, propertyDiagnostics);
  }
  return { children, propertyDiagnostics };
}

function inspectDescriptor(descriptor, path, code, children, diagnostics) {
  if (!descriptor.enumerable || !("value" in descriptor)) {
    diagnostics.push(diagnostic(code, path, "an accessor or non-enumerable property"));
    return;
  }
  children.push({ path, value: descriptor.value });
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

function boundDiagnostic(code, path, received) {
  return {
    code,
    path,
    message: `${path} exceeds the bounded data-only inspection: ${received}.`,
    expectation:
      `${DATA_ONLY_EXPECTATION} within ${MAXIMUM_DATA_ONLY_NODES} nodes and ${MAXIMUM_DATA_ONLY_DEPTH} levels`,
  };
}
