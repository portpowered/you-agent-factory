export interface FactorySimpleSubmissionWorkType {
  handlingBehavior?: readonly string[];
  isSubmitEligible: boolean;
  name: string;
}

export type FactorySimpleSubmissionFactoryState =
  | "active"
  | "closed"
  | "error"
  | "invalid"
  | "loading";

export type FactorySimpleSubmissionAvailability =
  | { kind: "available"; workTypeName: string }
  | {
      kind: "unavailable";
      reason:
        | "ambiguous-default"
        | "closed"
        | "error"
        | "history"
        | "invalid"
        | "loading"
        | "no-default";
    };

export interface FactorySimpleSubmissionEligibilityInput {
  factoryState: FactorySimpleSubmissionFactoryState;
  isCurrent: boolean;
  workTypes: readonly FactorySimpleSubmissionWorkType[];
}

export function resolveFactorySimpleSubmissionAvailability({
  factoryState,
  isCurrent,
  workTypes,
}: FactorySimpleSubmissionEligibilityInput): FactorySimpleSubmissionAvailability {
  if (!isCurrent) return { kind: "unavailable", reason: "history" };
  if (factoryState !== "active") {
    return { kind: "unavailable", reason: factoryState };
  }

  const defaultWorkTypes = workTypes.filter(
    (workType) =>
      workType.isSubmitEligible &&
      workType.handlingBehavior?.includes("DEFAULT"),
  );

  if (defaultWorkTypes.length === 0) {
    return { kind: "unavailable", reason: "no-default" };
  }
  if (defaultWorkTypes.length > 1) {
    return { kind: "unavailable", reason: "ambiguous-default" };
  }

  return { kind: "available", workTypeName: defaultWorkTypes[0].name };
}
