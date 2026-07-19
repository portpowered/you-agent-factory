export const FACTORY_EMULATOR_SCENARIO_SCHEMA_VERSION =
  "factory-emulator-scenario/v1" as const;

export interface FactoryEmulatorRuleSelector {
  readonly workstation?: string;
  readonly worker?: string;
  readonly input?: {
    readonly workType?: string;
    readonly state?: string;
    readonly name?: string;
  };
}

export interface FactoryEmulatorOutcome {
  readonly result: "accepted" | "continued" | "rejected" | "failed";
  readonly durationMs: number;
  readonly output?: string;
  readonly feedback?: string;
  readonly activityLabel?: string;
  readonly error?: string;
}

export interface FactoryEmulatorRule {
  readonly id: string;
  readonly selector: FactoryEmulatorRuleSelector;
  readonly cursor: {
    readonly scope: "lineage";
    readonly input: "rootWorkId";
  };
  readonly outcomes: readonly FactoryEmulatorOutcome[];
  readonly exhaustion: "repeat-last" | "fail";
}

export interface FactoryEmulatorInitialSubmission {
  readonly name: string;
  readonly workType: string;
  readonly state: string;
  readonly input?: string;
  readonly parent?: string;
}

export interface FactoryEmulatorSubmissionRelation {
  readonly type: "DEPENDS_ON" | "PARENT_CHILD" | "SPAWNED_BY";
  readonly sourceWorkName: string;
  readonly targetWorkName: string;
  readonly requiredState?: string;
}

export interface FactoryEmulatorSubmissionBatch {
  readonly works: readonly FactoryEmulatorInitialSubmission[];
  readonly relations?: readonly FactoryEmulatorSubmissionRelation[];
}

export type FactoryEmulatorInitialSubmissions =
  | readonly FactoryEmulatorInitialSubmission[]
  | FactoryEmulatorSubmissionBatch;

export interface FactoryEmulatorScenario {
  readonly schemaVersion: typeof FACTORY_EMULATOR_SCENARIO_SCHEMA_VERSION;
  readonly id: string;
  readonly factory: { readonly name: string };
  readonly seed: string;
  readonly startAt: string;
  readonly rules: readonly FactoryEmulatorRule[];
  readonly unmatched:
    | { readonly behavior: "error" }
    | {
        readonly behavior: "outcome";
        readonly outcome: FactoryEmulatorOutcome;
      };
  readonly initialSubmissions?: FactoryEmulatorInitialSubmissions;
}

export type FactoryEmulatorScenarioIssueCode =
  | "duplicate_identity"
  | "fully_shadowed_rule"
  | "invalid_cursor"
  | "invalid_exhaustion"
  | "invalid_factory_identity"
  | "invalid_initial_submission_relationship"
  | "invalid_outcome"
  | "invalid_selector_reference"
  | "invalid_start_at"
  | "invalid_type"
  | "invalid_unmatched"
  | "invalid_value"
  | "missing_required_field"
  | "unsupported_field"
  | "unsupported_schema_version"
  | "unstable_identity";

export interface FactoryEmulatorScenarioIssue {
  readonly category: "structure" | "semantic";
  readonly code: FactoryEmulatorScenarioIssueCode;
  readonly path: readonly (string | number)[];
  readonly message: string;
}

export type SafeParseFactoryEmulatorScenarioResult =
  | { readonly success: true; readonly data: FactoryEmulatorScenario }
  | {
      readonly success: false;
      readonly issues: readonly FactoryEmulatorScenarioIssue[];
    };

export class FactoryEmulatorScenarioValidationError extends Error {
  readonly issues: readonly FactoryEmulatorScenarioIssue[];

  constructor(issues: readonly FactoryEmulatorScenarioIssue[]) {
    super(
      issues.length === 1
        ? `Factory emulator scenario validation failed: ${issues[0]?.message}`
        : `Factory emulator scenario validation failed with ${issues.length} issues`,
    );
    this.name = "FactoryEmulatorScenarioValidationError";
    this.issues = issues;
  }
}
