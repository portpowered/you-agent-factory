import type { DashboardSnapshot } from "../../../api/dashboard";
import type { FactoryDefinition } from "../../../api/events";

import type { FactorySimpleSubmissionEligibilityInput } from "./factory-simple-submission-eligibility";

/**
 * Bridges the dashboard's projection-only submit list with the generated
 * Factory definition, which is the canonical source of DEFAULT behavior.
 */
export function adaptFactorySimpleSubmissionHost(input: {
  factory: FactoryDefinition | null | undefined;
  factoryState: string | undefined;
  isCurrent: boolean;
  submitWorkTypes: DashboardSnapshot["topology"]["submit_work_types"];
}): FactorySimpleSubmissionEligibilityInput {
  const submitEligibleNames = new Set(
    input.submitWorkTypes?.map((workType) => workType.work_type_name) ?? [],
  );

  return {
    factoryState: normalizeFactorySimpleSubmissionState(input.factoryState),
    isCurrent: input.isCurrent,
    workTypes: (input.factory?.workTypes ?? []).map((workType) => ({
      handlingBehavior: workType.handlingBehavior,
      isSubmitEligible: submitEligibleNames.has(workType.name),
      name: workType.name,
    })),
  };
}

function normalizeFactorySimpleSubmissionState(
  factoryState: string | undefined,
): FactorySimpleSubmissionEligibilityInput["factoryState"] {
  switch (factoryState) {
    case "IDLE":
    case "RUNNING":
      return "active";
    case "CLOSED":
      return "closed";
    case "ERROR":
      return "error";
    case "INVALID":
      return "invalid";
    default:
      return "loading";
  }
}
