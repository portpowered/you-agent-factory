export interface EmulatorSupportInspection {
  readonly scenarioVersion: string;
  readonly factory: { readonly supported: { readonly orchestrator: { readonly kind: "PETRI"; readonly allowOmitted: true }; readonly workstations: { readonly behavior: "STANDARD"; readonly allowOmitted: true } }; readonly unsupported: readonly string[] };
  readonly ruleMatchers: readonly ("all" | "workType" | "submissionId")[];
  readonly outcomeVariants: readonly ("complete" | "reject")[];
  readonly outcomeDuration: { readonly field: "durationMs"; readonly unit: "virtual milliseconds"; readonly default: 0 };
  readonly lineageCursors: { readonly initialSubmission: string; readonly scriptedOutcome: string };
  readonly exhaustionBehaviors: readonly ("repeatLast" | "useUnmatchedBehavior" | "reject")[];
  readonly unmatchedBehaviors: readonly ("ignore" | "reject")[];
  readonly initialSubmissions: { readonly requiredFields: readonly ("id" | "workType")[]; readonly workTypeMustExistInFactory: true };
  readonly activityLabel: { readonly maximumLength: 120; readonly transient: true; readonly canonicalFactoryEventField: false };
}

/** Returns the stable, machine-readable v1 emulator support report. */
export declare function inspectEmulatorSupport(): EmulatorSupportInspection;
