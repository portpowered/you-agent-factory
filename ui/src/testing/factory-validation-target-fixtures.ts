import type { FactoryValidationTarget } from "../api/factory-validation";

export function workerFieldValidationTarget(
  fieldName: string,
  message = `Invalid ${fieldName}.`,
): FactoryValidationTarget {
  return {
    code: `factory.worker.${fieldName}`,
    message,
    severity: "error",
    subject: {
      id: fieldName,
      location: "DEFINITION",
      type: "WORKER",
    },
  };
}

export function staleFactoryVersionTarget(
  message = "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
): FactoryValidationTarget {
  return {
    code: "factory.version.stale",
    message,
    severity: "error",
    subject: {
      id: "",
      location: "DEFINITION",
      type: "FACTORY",
    },
  };
}

export function factoryRuntimeNotIdleTarget(
  message = "Current factory runtime must be idle before activation.",
): FactoryValidationTarget {
  return {
    code: "factory.runtime.notIdle",
    message,
    severity: "error",
    subject: {
      id: "",
      location: "DEFINITION",
      type: "FACTORY",
    },
  };
}

export function factorySessionFieldTarget(
  reason: string,
  field: string,
  message: string,
): FactoryValidationTarget {
  return {
    code: `factory.session.field.${reason}`,
    message,
    severity: "error",
    subject: {
      id: field,
      location: "REFERENCE",
      type: "FACTORY",
    },
  };
}
