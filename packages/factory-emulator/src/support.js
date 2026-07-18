/**
 * The v1 browser-emulator support policy. Parsing consumes this policy directly,
 * and inspection exposes it without requiring callers to reproduce parser rules.
 */
export const emulatorSupport = deepFreeze({
  scenarioVersion: "you-agent-factory.emulator.scenario.v1",
  factory: {
    supported: {
      orchestrator: { kind: "PETRI", allowOmitted: true },
      workstations: { behavior: "STANDARD", allowOmitted: true },
    },
    unsupported: [
      "JavaScript orchestration",
      "configured Factory resources",
      "Factory guards",
      "cron workstation scheduling",
      "non-STANDARD workstation behavior",
    ],
  },
  ruleMatchers: ["all", "workType", "submissionId"],
  outcomeVariants: ["complete", "reject"],
  lineageCursors: {
    initialSubmission: "one unique initial submission id",
    scriptedOutcome: "one earlier complete scripted outcome",
  },
  exhaustionBehaviors: ["repeatLast", "useUnmatchedBehavior", "reject"],
  unmatchedBehaviors: ["ignore", "reject"],
  initialSubmissions: {
    requiredFields: ["id", "workType"],
    workTypeMustExistInFactory: true,
  },
  activityLabel: {
    maximumLength: 120,
    transient: true,
    canonicalFactoryEventField: false,
  },
});

/** Returns the stable, machine-readable v1 emulator support report. */
export function inspectEmulatorSupport() {
  return emulatorSupport;
}

/** Returns Factory-subset diagnostics used by the public parser. */
export function factorySupportDiagnostics(factory) {
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
  if (
    factory.orchestrator.kind !== undefined &&
    factory.orchestrator.kind !== emulatorSupport.factory.supported.orchestrator.kind
  ) {
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
    if (
      workstation.behavior !== undefined &&
      workstation.behavior !== emulatorSupport.factory.supported.workstations.behavior
    ) {
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

function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function deepFreeze(value) {
  Object.freeze(value);
  for (const child of Object.values(value)) {
    if (typeof child === "object" && child !== null && !Object.isFrozen(child)) {
      deepFreeze(child);
    }
  }
  return value;
}
