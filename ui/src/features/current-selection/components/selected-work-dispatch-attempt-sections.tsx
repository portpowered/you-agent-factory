import { type ReactNode, useId, useState } from "react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../components/ui/widget-frame";
import { formatDurationMillis } from "../../../components/ui/formatters";
import type {
  DashboardInferenceAttempt,
  DashboardScriptRequest,
  DashboardScriptResponse,
} from "../../../api/dashboard/types";
import {
  EXECUTION_PILL_CLASS,
  HISTORY_HEADER_CLASS,
  HISTORY_TOGGLE_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
} from "./detail-card-shared";
import { InferenceAttemptCard } from "./inference-attempt";
import type { SelectedWorkRequestHistoryItem } from "./detail-card-types";
import {
  useCurrentSelectionDispatchHistoryMessages,
  useCurrentSelectionOperationalEnumMessages,
  useCurrentSelectionLocale,
} from "./current-selection-locale";
import type { LoadableProviderSessionRef } from "../../provider-session-detail/lib/provider-session-ref";
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
} from "../dispatch-history/selected-work-dispatch-history-helpers";
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
          <p className={DETAIL_COPY_CLASS}>{emptyCopy}</p>
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
          <p className={DETAIL_COPY_CLASS}>
            {messages.noScriptAttemptRecordedYet}
          </p>
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
  const [expanded, setExpanded] = useState(false);
  const sectionId = useId();
  const panelId = `${sectionId}-panel`;
  const headingId = `${sectionId}-heading`;

  return (
    <section
      aria-labelledby={headingId}
      className="mt-3 grid gap-2.5 border-t border-af-border pt-3"
    >
      <div className={HISTORY_HEADER_CLASS}>
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={headingId}>
          {title}
        </h4>
        <button
          aria-controls={panelId}
          aria-expanded={expanded}
          className={HISTORY_TOGGLE_CLASS}
          onClick={() => setExpanded((current) => !current)}
          type="button"
        >
          {expanded ? messages.collapseAction : messages.expandAction}
        </button>
      </div>
      {expanded ? <div id={panelId}>{children}</div> : null}
    </section>
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
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong>
            {messages.requestAttemptLabel(
              String(attemptNumber ?? messages.pendingAttemptLabel),
            )}
          </strong>
          <p className={`m-0 text-af-text-muted ${DASHBOARD_BODY_TEXT_CLASS}`}>
            {enumMessages.localizeOutcome("PENDING")}
          </p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>
          {requestID ?? messages.scriptRequestPlaceholderId}
        </span>
      </div>
      <dl className={`mt-2.5 ${INFERENCE_ATTEMPT_DETAIL_CLASS}`}>
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
      </dl>
      <ScriptArgsSection
        args={scriptRequest.args}
        label={messages.resolvedArgsLabel}
      />
    </article>
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
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong>
            {messages.responseAttemptLabel(
              String(attemptNumber ?? messages.completedAttemptLabel),
            )}
          </strong>
          <p className={`m-0 text-af-text-muted ${DASHBOARD_BODY_TEXT_CLASS}`}>
            {scriptResponse.outcome
              ? enumMessages.localizeOutcome(scriptResponse.outcome)
              : enumMessages.localizeOutcome("RECORDED")}
          </p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>
          {requestID ?? messages.scriptResponsePlaceholderId}
        </span>
      </div>
      <dl className={`mt-2.5 ${INFERENCE_ATTEMPT_DETAIL_CLASS}`}>
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
      </dl>
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
    </article>
  );
}
