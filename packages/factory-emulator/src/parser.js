import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import { scenarioSchema } from "./generated/scenario-schema.js";
import { factorySchema } from "./generated/factory-schema.js";
import { scenarioSemanticDiagnostics } from "./semantics.js";
import { factorySupportDiagnostics } from "./support.js";
import { dataOnlyDiagnostics } from "./data-only.js";

const ajv = new Ajv2020({
  allErrors: true,
  coerceTypes: false,
  removeAdditional: false,
  strict: false,
  useDefaults: false,
});
addFormats(ajv);
ajv.addFormat("int32", true);
ajv.addFormat("int64", true);
const validateScenarioShape = ajv.compile(scenarioSchema);
const validateFactoryShape = ajv.compile(factorySchema);

/**
 * Parses only data. It neither creates Factory events nor starts emulator
 * activity, so callers can reject unsupported input before any runtime work.
 */
export function parseEmulatorScenario(scenario, factory) {
  const scenarioDataDiagnostics = dataOnlyDiagnostics(scenario, {
    code: "INVALID_SCENARIO_SHAPE",
  });
  if (scenarioDataDiagnostics.length > 0) {
    return failure(scenarioDataDiagnostics);
  }
  if (!validateScenarioShape(scenario)) {
    return failure(shapeDiagnostics(validateScenarioShape.errors ?? []));
  }

  const factoryDiagnostics = dataOnlyDiagnostics(factory, {
    code: "INVALID_FACTORY_DEFINITION",
  });
  if (factoryDiagnostics.length === 0 && !validateFactoryShape(factory)) {
    factoryDiagnostics.push(
      ...shapeDiagnostics(
        validateFactoryShape.errors ?? [],
        "INVALID_FACTORY_DEFINITION",
      ),
    );
  }
  if (factoryDiagnostics.length === 0) {
    factoryDiagnostics.push(...factorySupportDiagnostics(factory));
  }
  const diagnostics =
    factoryDiagnostics.length === 0
      ? scenarioSemanticDiagnostics(scenario, factory)
      : factoryDiagnostics;
  return diagnostics.length === 0
    ? { success: true, scenario, factory }
    : failure(diagnostics);
}

function failure(diagnostics) {
  return { success: false, diagnostics: Object.freeze(diagnostics) };
}

function shapeDiagnostics(errors, defaultCode = "INVALID_SCENARIO_SHAPE") {
  return errors
    .map((error) => {
      const path = errorPath(error);
      return {
        code:
          error.keyword === "const" && path === "/version"
            ? "UNSUPPORTED_SCENARIO_VERSION"
            : defaultCode,
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

function compareDiagnostics(left, right) {
  return (
    left.path.localeCompare(right.path) ||
    left.code.localeCompare(right.code) ||
    left.expectation.localeCompare(right.expectation)
  );
}

function escapeJsonPointer(value) {
  return value.replaceAll("~", "~0").replaceAll("/", "~1");
}
