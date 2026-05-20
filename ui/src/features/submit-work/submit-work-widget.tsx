import type { DashboardSubmitWorkType } from "../../api/dashboard/types";
import { useDashboardSessionStore } from "../dashboard/state/dashboardSessionStore";
import { getSubmitWorkMessages } from "./messages/submit-work";
import { SubmitWorkCard } from "./submit-work-card";
import { useSubmitWorkWidget } from "./use-submit-work-widget";

export interface SubmitWorkWidgetProps {
  locale?: string;
  submitWorkTypes?: DashboardSubmitWorkType[];
}

export function SubmitWorkWidget({
  locale,
  submitWorkTypes = [],
}: SubmitWorkWidgetProps) {
  const selectedSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const messages = getSubmitWorkMessages(locale);
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
  } = useSubmitWorkWidget(selectedSessionID, submitWorkTypes, messages);

  return (
    <SubmitWorkCard
      draft={draft}
      isSubmitting={isSubmitting}
      locale={locale}
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
