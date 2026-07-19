import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import type { DashboardSubmitWorkType } from "../../../api/dashboard/types";
import { submitWork } from "../../../api/work";
import { useCurrentFactoryDefinition } from "../../current-factory-definition/public";
import { useDashboardSession } from "../../dashboard/session/dashboard-session-provider";
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
  const [simpleDraft, setSimpleDraft] = useState("");
  const simpleMutation = useMutation({
    mutationFn: (submission: {
      content: readonly [{ text: string; type: "text" }];
      workTypeName: string;
    }) =>
      submitWork(
        {
          content: [...submission.content],
          workTypeName: submission.workTypeName,
        },
        { sessionID },
      ),
  });
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
        draft={simpleDraft}
        isSubmitting={simpleMutation.isPending}
        locale={locale}
        onDraftChange={(value) => {
          simpleMutation.reset();
          setSimpleDraft(value);
        }}
        onSubmit={async (submission) => {
          await simpleMutation.mutateAsync(submission);
        }}
        submissionError={
          simpleMutation.isError && simpleMutation.error instanceof Error
            ? simpleMutation.error.message
            : undefined
        }
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
