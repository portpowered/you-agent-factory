import type { ReactNode } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { SubmitWorkCard } from "./submit-work-card";
import { useSubmitWorkWidget } from "../hooks/use-submit-work-widget";

export interface SubmitWorkWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  submitWorkTypes?: DashboardSubmitWorkType[];
}

export function SubmitWorkWidget({
  headerAction,
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
  } = useSubmitWorkWidget(
    selectedSessionID ?? DEFAULT_FACTORY_SESSION_ID,
    submitWorkTypes,
    messages,
  );

  return (
    <SubmitWorkCard
      draft={draft}
      headerAction={headerAction}
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
