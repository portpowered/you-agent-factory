import type { WorkstationDetailCardProps } from "../lib/detail-card-types";
import type { getWorkstationDetailMessages } from "../messages/workstation-detail";

export function resolveWorkstationSummaryTypeValue(
  state: WorkstationDetailCardProps["editableConfigurationState"],
  messages: ReturnType<typeof getWorkstationDetailMessages>,
): string {
  if (!state || state.status === "loading") {
    return messages.workstationTypeLoadingValue;
  }

  if (state.status === "error" || state.status === "empty") {
    return messages.unavailableWorkstationTypeValue;
  }

  return messages.localizeWorkstationType(state.initialValues.workstationType);
}

export { resolveWorkstationSummaryRunnerValue } from "./workstation-runner-field";
