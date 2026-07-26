import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { type ReactNode, useId } from "react";
import type {
  DashboardInferenceAttempt,
  DashboardScriptRequest,
  DashboardScriptResponse,
} from "../../../../../api/dashboard/types";
import { formatDurationMillis } from "../../../../../components/ui/formatters";
import type { LoadableProviderSessionRef } from "../../../../provider-session-detail/lib/provider-session-ref";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionOperationalEnumMessages,
} from "../../../base/components/presentation/current-selection-locale";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import {
  CurrentSelectionHistoryCard,
  CurrentSelectionHistoryCardHeader,
} from "../../../history/components/current-selection-history-card";
import { InferenceAttemptCard } from "../../../work-selection/components/inference-attempt/inference-attempt";
import { InferenceAttemptDetail } from "../../../work-selection/components/inference-attempt/inference-attempt-detail";
import {
  requestModel,
  requestProvider,
  requestWorkingDirectory,
  requestWorktree,
  scriptAttemptNumber,
  scriptRequestID,
  scriptResponseDurationMillis,
  scriptResponseExitCode,
  scriptResponseFailureType,
} from "../../dispatch-history/selected-work-dispatch-history-helpers";
import type { SelectedWorkRequestHistoryItem } from "../../lib/detail-card-types";
import {
  ScriptArgsSection,
  ScriptOutputSection,
} from "./selected-work-dispatch-history-card-shared";

export function DispatchInferenceAttemptsSection({
  attempts,
  emptyCopy,
  onSelectProviderSession,
  selectedProviderSessionKey,
}: {
  attempts: DashboardInferenceAttempt[];
  emptyCopy?: string;
  onSelectProviderSession?: (session: LoadableProviderSessionRef) => void;
  selectedProviderSessionKey?: string | null;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <CollapsibleDispatchAttemptSection title={messages.inferenceAttemptsTitle}>
      <div className="grid gap-2.5">
        {attempts.length > 0 ? (
          attempts.map((attempt) => (
            <InferenceAttemptCard
              attempt={attempt}
              key={attempt.inference_request_id}
              onSelectProviderSession={onSelectProviderSession}
              selectedProviderSessionKey={selectedProviderSessionKey}
            />
          ))
        ) : emptyCopy ? (
          <WidgetDetailCopy>{emptyCopy}</WidgetDetailCopy>
        ) : null}
      </div>
    </CollapsibleDispatchAttemptSection>
  );
}

export function DispatchScriptAttemptsSection({
  normalizedStderr,
  normalizedStdout,
  request,
  scriptRequest,
  scriptResponse,
}: {
  normalizedStderr: string | undefined;
  normalizedStdout: string | undefined;
  request: SelectedWorkRequestHistoryItem;
  scriptRequest: DashboardScriptRequest | undefined;
  scriptResponse: DashboardScriptResponse | undefined;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <CollapsibleDispatchAttemptSection title={messages.scriptAttemptsTitle}>
      <div className="grid gap-2.5">
        {scriptRequest ? (
          <ScriptRequestAttemptCard
            request={request}
            scriptRequest={scriptRequest}
          />
        ) : null}
        {scriptResponse ? (
          <ScriptResponseAttemptCard
            fallbackAttemptNumber={scriptAttemptNumber(scriptRequest)}
            normalizedStderr={normalizedStderr}
            normalizedStdout={normalizedStdout}
            request={request}
            scriptResponse={scriptResponse}
          />
        ) : (
          <WidgetDetailCopy>
            {messages.noScriptAttemptRecordedYet}
          </WidgetDetailCopy>
        )}
      </div>
    </CollapsibleDispatchAttemptSection>
  );
}

function CollapsibleDispatchAttemptSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const sectionId = useId();
  const panelId = `${sectionId}-panel`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      className="mt-3 border-t border-outline pt-3"
      contentId={panelId}
      headingId={headingId}
      title={title}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {children}
    </CurrentSelectionExpandableSection>
  );
}

function ScriptRequestAttemptCard({
  request,
  scriptRequest,
}: {
  request: SelectedWorkRequestHistoryItem;
  scriptRequest: DashboardScriptRequest;
}) {
  const attemptNumber = scriptAttemptNumber(scriptRequest);
  const requestID = scriptRequestID(scriptRequest);
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();

  return (
    <CurrentSelectionHistoryCard>
      <CurrentSelectionHistoryCardHeader
        identifier={requestID ?? messages.scriptRequestPlaceholderId}
        subtitle={enumMessages.localizeOutcome("PENDING")}
        title={messages.requestAttemptLabel(
          String(attemptNumber ?? messages.pendingAttemptLabel),
        )}
      />
      <CurrentSelectionDescriptionList className="mt-2.5">
        <InferenceAttemptDetail
          label={messages.scriptRequestIdLabel}
          code
          value={requestID}
        />
        <InferenceAttemptDetail
          label={messages.scriptAttemptLabel}
          value={
            attemptNumber !== undefined ? String(attemptNumber) : undefined
          }
        />
        <InferenceAttemptDetail
          label={messages.providerLabel}
          code
          value={requestProvider(request)}
        />
        <InferenceAttemptDetail
          label={messages.modelLabel}
          code
          value={requestModel(request)}
        />
        <InferenceAttemptDetail
          label={messages.workingDirectoryLabel}
          code
          value={requestWorkingDirectory(request)}
        />
        <InferenceAttemptDetail
          label={messages.worktreeLabel}
          code
          value={requestWorktree(request)}
        />
        <InferenceAttemptDetail
          label={messages.commandLabel}
          code
          value={scriptRequest.command}
        />
      </CurrentSelectionDescriptionList>
      <ScriptArgsSection
        args={scriptRequest.args}
        label={messages.resolvedArgsLabel}
      />
    </CurrentSelectionHistoryCard>
  );
}

function ScriptResponseAttemptCard({
  fallbackAttemptNumber,
  normalizedStderr,
  normalizedStdout,
  request,
  scriptResponse,
}: {
  fallbackAttemptNumber: number | undefined;
  normalizedStderr: string | undefined;
  normalizedStdout: string | undefined;
  request: SelectedWorkRequestHistoryItem;
  scriptResponse: DashboardScriptResponse;
}) {
  const attemptNumber =
    scriptAttemptNumber(scriptResponse) ?? fallbackAttemptNumber;
  const requestID = scriptRequestID(scriptResponse);
  const durationMillis = scriptResponseDurationMillis(scriptResponse);
  const exitCode = scriptResponseExitCode(scriptResponse);
  const failureType = scriptResponseFailureType(scriptResponse);
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const enumMessages = useCurrentSelectionOperationalEnumMessages();
  const locale = useCurrentSelectionLocale();

  return (
    <CurrentSelectionHistoryCard>
      <CurrentSelectionHistoryCardHeader
        identifier={requestID ?? messages.scriptResponsePlaceholderId}
        subtitle={
          scriptResponse.outcome
            ? enumMessages.localizeOutcome(scriptResponse.outcome)
            : enumMessages.localizeOutcome("RECORDED")
        }
        title={messages.responseAttemptLabel(
          String(attemptNumber ?? messages.completedAttemptLabel),
        )}
      />
      <CurrentSelectionDescriptionList className="mt-2.5">
        <InferenceAttemptDetail
          label={messages.scriptRequestIdLabel}
          code
          value={requestID}
        />
        <InferenceAttemptDetail
          label={messages.scriptAttemptLabel}
          value={
            attemptNumber !== undefined ? String(attemptNumber) : undefined
          }
        />
        <InferenceAttemptDetail
          label={messages.providerLabel}
          code
          value={requestProvider(request)}
        />
        <InferenceAttemptDetail
          label={messages.modelLabel}
          code
          value={requestModel(request)}
        />
        <InferenceAttemptDetail
          label={messages.workingDirectoryLabel}
          code
          value={requestWorkingDirectory(request)}
        />
        <InferenceAttemptDetail
          label={messages.worktreeLabel}
          code
          value={requestWorktree(request)}
        />
        <InferenceAttemptDetail
          label={messages.outcomeLabel}
          value={
            scriptResponse.outcome
              ? enumMessages.localizeOutcome(scriptResponse.outcome)
              : undefined
          }
        />
        <InferenceAttemptDetail
          label={messages.durationLabel}
          value={
            durationMillis !== undefined
              ? formatDurationMillis(durationMillis, locale)
              : undefined
          }
        />
        <InferenceAttemptDetail
          label={messages.exitCodeLabel}
          value={exitCode !== undefined ? String(exitCode) : undefined}
        />
        <InferenceAttemptDetail
          label={messages.failureTypeLabel}
          code
          value={failureType}
        />
      </CurrentSelectionDescriptionList>
      <ScriptOutputSection
        emptyMessage={messages.noStdoutRecorded}
        label={messages.stdoutLabel}
        value={normalizedStdout}
      />
      <ScriptOutputSection
        emptyMessage={messages.noStderrRecorded}
        label={messages.stderrLabel}
        value={normalizedStderr}
      />
    </CurrentSelectionHistoryCard>
  );
}
