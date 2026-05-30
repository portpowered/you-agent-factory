import type { ReactNode } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
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
  const { sessionID } = useDashboardSession();
  const messages = getSubmitWorkMessages(locale);
  const {
    draft,
    isSubmitting,
    onAddItem,
    onItemTextChange,
    onRemoveItem,
    onRequestNameChange,
    onStageFileItems,
    onSubmit,
    onWorkTypeNameChange,
    status,
    submitWorkTypeNames,
    validationErrors,
  } = useSubmitWorkWidget(sessionID, submitWorkTypes, messages);

  return (
    <SubmitWorkCard
      draft={draft}
      headerAction={headerAction}
      isSubmitting={isSubmitting}
      locale={locale}
      onAddItem={onAddItem}
      onItemTextChange={onItemTextChange}
      onRemoveItem={onRemoveItem}
      onRequestNameChange={onRequestNameChange}
      onStageFileItems={onStageFileItems}
      onSubmit={onSubmit}
      onWorkTypeNameChange={onWorkTypeNameChange}
      status={status}
      submitWorkTypeNames={submitWorkTypeNames}
      validationErrors={validationErrors}
    />
  );
}
