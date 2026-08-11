import type { ReactNode } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { useCurrentFactoryDefinition } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
import { useSessionScopedSimpleSubmission } from "../hooks/use-session-scoped-simple-submission";
import { useSubmitWorkWidget } from "../hooks/use-submit-work-widget";
import { adaptFactorySimpleSubmissionHost } from "../lib/factory-simple-submission-host-adapter";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { FactorySimpleSubmissionComposer } from "./composer/factory-simple-submission-composer";
import { FactorySubmissionComposer } from "./composer/factory-submission-composer";
import { FactoryInvocationWidget } from "./invocation/factory-invocation-widget";

export interface SubmitWorkWidgetProps {
  headerAction?: ReactNode;
  locale?: string;
  factoryState?: string;
  isCurrent?: boolean;
  submitWorkTypes?: DashboardSubmitWorkType[];
}

export function SubmitWorkWidget({
  headerAction,
  factoryState,
  isCurrent = true,
  locale,
  submitWorkTypes = [],
}: SubmitWorkWidgetProps) {
  const { sessionID } = useDashboardSession();
  const messages = getSubmitWorkMessages(locale);
  const currentFactory = useCurrentFactoryDefinition();
  const simpleSubmission = useSessionScopedSimpleSubmission(
    sessionID,
    messages,
  );

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

  const simpleHost = adaptFactorySimpleSubmissionHost({
    factory: currentFactory.data,
    factoryState,
    isCurrent,
    submitWorkTypes,
  });
  const hasDefaultWorkType = currentFactory.data?.workTypes?.some((workType) =>
    workType.handlingBehavior?.includes("DEFAULT"),
  );

  if (hasDefaultWorkType) {
    return (
      <FactorySimpleSubmissionComposer
        {...simpleHost}
        draft={simpleSubmission.draft}
        isSubmitting={simpleSubmission.isSubmitting}
        locale={locale}
        onDraftChange={simpleSubmission.onDraftChange}
        onSubmit={simpleSubmission.onSubmit}
        sessionID={sessionID}
        submissionError={simpleSubmission.submissionError}
        submissionSuccess={simpleSubmission.submissionSuccess}
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
      sessionID={sessionID}
      status={status}
      submitWorkTypeNames={submitWorkTypeNames}
      validationErrors={validationErrors}
    />
  );
}
