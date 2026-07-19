import type { ReactNode } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { useCurrentFactoryDefinition } from "../../current-factory-definition/public";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { useSubmitWorkWidget } from "../hooks/use-submit-work-widget";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { FactorySubmissionComposer } from "./factory-submission-composer";
import { FactoryInvocationWidget } from "./invocation/factory-invocation-widget";

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
  const currentFactory = useCurrentFactoryDefinition();
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

  if (currentFactory.data?.invocationSignature) {
    return (
      <FactoryInvocationWidget
        headerAction={headerAction}
        locale={locale}
        sessionID={sessionID}
      />
    );
  }

  return (
    <FactorySubmissionComposer
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
