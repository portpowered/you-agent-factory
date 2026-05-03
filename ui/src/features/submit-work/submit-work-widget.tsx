import type { DashboardSubmitWorkType } from "../../api/dashboard/types";
import { SubmitWorkCard } from "./submit-work-card";
import { useSubmitWorkWidget } from "./use-submit-work-widget";

export interface SubmitWorkWidgetProps {
  submitWorkTypes?: DashboardSubmitWorkType[];
}

export function SubmitWorkWidget({ submitWorkTypes = [] }: SubmitWorkWidgetProps) {
  const {
    draft,
    isSubmitting,
    onRequestNameChange,
    onRequestTextChange,
    onSubmit,
    onWorkTypeNameChange,
    status,
    submitWorkTypeNames,
    validationErrors,
  } = useSubmitWorkWidget(submitWorkTypes);

  return (
    <SubmitWorkCard
      draft={draft}
      isSubmitting={isSubmitting}
      onRequestNameChange={onRequestNameChange}
      onRequestTextChange={onRequestTextChange}
      onSubmit={onSubmit}
      onWorkTypeNameChange={onWorkTypeNameChange}
      status={status}
      submitWorkTypeNames={submitWorkTypeNames}
      validationErrors={validationErrors}
    />
  );
}

