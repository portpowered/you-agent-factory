import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { scenarioSchema } from "./generated/scenario-schema.js";

const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
addFormats(ajv);
const validateScenarioShape = ajv.compile(scenarioSchema);

/**
 * Parses only data. It neither creates Factory events nor starts emulator
 * activity, so callers can reject unsupported input before any runtime work.
 */
export function parseEmulatorScenario(scenario, factory) {
  if (!validateScenarioShape(scenario)) {
    return failure(shapeDiagnostics(validateScenarioShape.errors ?? []));
  }

  const diagnostics = factorySupportDiagnostics(factory);
  return diagnostics.length === 0
    ? { success: true, scenario, factory }
    : failure(diagnostics);
}

function failure(diagnostics) {
  return { success: false, diagnostics: Object.freeze(diagnostics) };
}

function shapeDiagnostics(errors) {
  return errors
    .map((error) => {
      const path = errorPath(error);
      return {
        code:
          error.keyword === "const" && path === "/version"
            ? "UNSUPPORTED_SCENARIO_VERSION"
            : "INVALID_SCENARIO_SHAPE",
        path,
        message: `${path} ${error.message ?? "does not match the Emulator Scenario schema"}.`,
        expectation: shapeExpectation(error),
      };
    })
    .sort(compareDiagnostics);
}

function errorPath(error) {
  const property =
    error.keyword === "required"
      ? error.params.missingProperty
      : error.keyword === "additionalProperties"
        ? error.params.additionalProperty
        : undefined;
  return typeof property === "string"
    ? `${error.instancePath}/${escapeJsonPointer(property)}`
    : error.instancePath || "/";
}

function shapeExpectation(error) {
  if (error.keyword === "required") {
    return `required property ${JSON.stringify(error.params.missingProperty)}`;
  }
  if (error.keyword === "additionalProperties") {
    return `no additional property ${JSON.stringify(error.params.additionalProperty)}`;
  }
  return error.message ?? error.keyword;
}

function factorySupportDiagnostics(factory) {
  if (!isRecord(factory)) {
    return [
      diagnostic(
        "INVALID_FACTORY_DEFINITION",
        "/",
        "Factory definition must be an object.",
        "a Factory definition object",
      ),
    ];
  }

  const diagnostics = [];
  validateOrchestrator(factory, diagnostics);
  rejectConfiguredCapability(factory, diagnostics, "resources", "resource capacity");
  rejectConfiguredCapability(factory, diagnostics, "guards", "Factory guards");
  validateWorkstations(factory, diagnostics);
  return diagnostics;
}

function validateOrchestrator(factory, diagnostics) {
  if (factory.orchestrator === undefined) {
    return;
  }
  if (!isRecord(factory.orchestrator)) {
    diagnostics.push(
      diagnostic(
        "INVALID_FACTORY_DEFINITION",
        "/orchestrator",
        "Factory orchestrator must be an object when supplied.",
        "a static PETRI orchestrator",
      ),
    );
    return;
  }
  if (factory.orchestrator.kind !== undefined && factory.orchestrator.kind !== "PETRI") {
    diagnostics.push(
      diagnostic(
        "UNSUPPORTED_FACTORY_CAPABILITY",
        "/orchestrator/kind",
        `Factory orchestrator ${JSON.stringify(factory.orchestrator.kind)} is not supported by the emulator.`,
        "a static PETRI orchestrator",
      ),
    );
  }
}

function rejectConfiguredCapability(factory, diagnostics, property, capability) {
  if (Array.isArray(factory[property]) && factory[property].length > 0) {
    diagnostics.push(
      diagnostic(
        "UNSUPPORTED_FACTORY_CAPABILITY",
        `/${property}`,
        `Factory ${capability} are not supported by the emulator.`,
        `no configured ${capability}`,
      ),
    );
  }
}

function validateWorkstations(factory, diagnostics) {
  if (factory.workstations === undefined) {
    return;
  }
  if (!Array.isArray(factory.workstations)) {
    diagnostics.push(
      diagnostic(
        "INVALID_FACTORY_DEFINITION",
        "/workstations",
        "Factory workstations must be an array when supplied.",
        "an array of static standard workstations",
      ),
    );
    return;
  }

  for (const [index, workstation] of factory.workstations.entries()) {
    const path = `/workstations/${index}`;
    if (!isRecord(workstation)) {
      diagnostics.push(
        diagnostic(
          "INVALID_FACTORY_DEFINITION",
          path,
          "Factory workstation must be an object.",
          "a static standard workstation object",
        ),
      );
      continue;
    }
    if (workstation.behavior !== undefined && workstation.behavior !== "STANDARD") {
      diagnostics.push(
        diagnostic(
          "UNSUPPORTED_FACTORY_CAPABILITY",
          `${path}/behavior`,
          `Factory workstation behavior ${JSON.stringify(workstation.behavior)} is not supported by the emulator.`,
          "STANDARD workstation behavior",
        ),
      );
    }
    if (workstation.cron !== undefined) {
      diagnostics.push(
        diagnostic(
          "UNSUPPORTED_FACTORY_CAPABILITY",
          `${path}/cron`,
          "Factory cron scheduling is not supported by the emulator.",
          "no cron scheduling",
        ),
      );
    }
  }
}

function diagnostic(code, path, message, expectation) {
  return { code, path, message, expectation };
}

function compareDiagnostics(left, right) {
  return (
    left.path.localeCompare(right.path) ||
    left.code.localeCompare(right.code) ||
    left.expectation.localeCompare(right.expectation)
  );
}

function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function escapeJsonPointer(value) {
  return value.replaceAll("~", "~0").replaceAll("/", "~1");
}
