import type { FactoryValidationTarget } from "../api/factory-validation";

export function workStateFieldValidationTarget(
  fieldName: string,
  message = `Invalid ${fieldName}.`,
): FactoryValidationTarget {
  return {
    code: `factory.workTypes[0].states[0].${fieldName}`,
    message,
    severity: "error",
    subject: {
      id: fieldName,
      location: "DEFINITION",
      type: "WORK_STATE",
    },
  };
}

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

export function resourceFieldValidationTarget(
  fieldName: string,
  message = `Invalid ${fieldName}.`,
): FactoryValidationTarget {
  return {
    code: `factory.resource.${fieldName}`,
    message,
    severity: "error",
    subject: {
      id: fieldName,
      location: "DEFINITION",
      type: "RESOURCE",
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

export function factorySessionTargetTarget(
  reason: string,
  targetID: string,
  message: string,
): FactoryValidationTarget {
  return {
    code: `factory.session.target.${reason}`,
    message,
    severity: "error",
    subject: {
      id: targetID,
      location: "REFERENCE",
      type: "FACTORY",
    },
  };
}
