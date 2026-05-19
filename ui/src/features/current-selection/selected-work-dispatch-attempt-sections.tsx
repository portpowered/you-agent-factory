import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import { formatDurationMillis } from "../../components/ui/formatters";
import type {
  DashboardInferenceAttempt,
  DashboardScriptRequest,
  DashboardScriptResponse,
} from "../../api/dashboard/types";
import {
  EXECUTION_PILL_CLASS,
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  PROVIDER_SESSION_CARD_CLASS,
} from "./detail-card-shared";
import { InferenceAttemptCard } from "./inference-attempt";
import type { SelectedWorkRequestHistoryItem } from "./detail-card-types";
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
} from "./selected-work-dispatch-history-helpers";
import {
  ScriptArgsSection,
  ScriptOutputSection,
} from "./selected-work-dispatch-history-card-shared";
import { useCurrentSelectionDispatchHistoryMessages } from "./current-selection-locale";

export function DispatchInferenceAttemptsSection({
  attempts,
  emptyCopy,
}: {
  attempts: DashboardInferenceAttempt[];
  emptyCopy?: string;
}) {
  const messages = useCurrentSelectionDispatchHistoryMessages();

  return (
    <section
      aria-label={messages.inferenceAttemptsHeading}
      className="mt-3 grid gap-2 border-t border-af-overlay/8 pt-3"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.inferenceAttemptsHeading}
      </h4>
      <div className="grid gap-2.5">
        {attempts.length > 0
          ? attempts.map((attempt) => (
              <InferenceAttemptCard attempt={attempt} key={attempt.inference_request_id} />
            ))
          : emptyCopy
            ? <p className={DETAIL_COPY_CLASS}>{emptyCopy}</p>
            : null}
      </div>
    </section>
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
    <section
      aria-label={messages.scriptAttemptsHeading}
      className="mt-3 grid gap-2 border-t border-af-overlay/8 pt-3"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.scriptAttemptsHeading}
      </h4>
      <div className="grid gap-2.5">
        {scriptRequest ? (
          <ScriptRequestAttemptCard request={request} scriptRequest={scriptRequest} />
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
          <p className={DETAIL_COPY_CLASS}>{messages.scriptAttemptsEmpty}</p>
        )}
      </div>
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
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const attemptNumber = scriptAttemptNumber(scriptRequest);
  const requestID = scriptRequestID(scriptRequest);

  return (
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong>{messages.requestAttemptTitle(attemptNumber)}</strong>
          <p className={`m-0 text-af-ink/70 ${DASHBOARD_BODY_TEXT_CLASS}`}>
            {messages.pendingOutcome}
          </p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>
          {requestID ?? messages.requestAttemptFallbackId}
        </span>
      </div>
      <dl className={`mt-2.5 ${INFERENCE_ATTEMPT_DETAIL_CLASS}`}>
        <InferenceAttemptDetail label={messages.scriptRequestIdLabel} code value={requestID} />
        <InferenceAttemptDetail
          label={messages.scriptAttemptLabel}
          value={attemptNumber !== undefined ? String(attemptNumber) : undefined}
        />
        <InferenceAttemptDetail label={messages.providerLabel} code value={requestProvider(request)} />
        <InferenceAttemptDetail label={messages.modelLabel} code value={requestModel(request)} />
        <InferenceAttemptDetail
          label={messages.workingDirectoryLabel}
          code
          value={requestWorkingDirectory(request)}
        />
        <InferenceAttemptDetail label={messages.worktreeLabel} code value={requestWorktree(request)} />
        <InferenceAttemptDetail label={messages.commandLabel} code value={scriptRequest.command} />
      </dl>
      <ScriptArgsSection args={scriptRequest.args} label={messages.resolvedArgsLabel} />
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
  const messages = useCurrentSelectionDispatchHistoryMessages();
  const attemptNumber = scriptAttemptNumber(scriptResponse) ?? fallbackAttemptNumber;
  const requestID = scriptRequestID(scriptResponse);
  const durationMillis = scriptResponseDurationMillis(scriptResponse);
  const exitCode = scriptResponseExitCode(scriptResponse);
  const failureType = scriptResponseFailureType(scriptResponse);

  return (
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <strong>{messages.responseAttemptTitle(attemptNumber)}</strong>
          <p className={`m-0 text-af-ink/70 ${DASHBOARD_BODY_TEXT_CLASS}`}>
            {scriptResponse.outcome ?? messages.recordedOutcome}
          </p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>
          {requestID ?? messages.responseAttemptFallbackId}
        </span>
      </div>
      <dl className={`mt-2.5 ${INFERENCE_ATTEMPT_DETAIL_CLASS}`}>
        <InferenceAttemptDetail label={messages.scriptRequestIdLabel} code value={requestID} />
        <InferenceAttemptDetail
          label={messages.scriptAttemptLabel}
          value={attemptNumber !== undefined ? String(attemptNumber) : undefined}
        />
        <InferenceAttemptDetail label={messages.providerLabel} code value={requestProvider(request)} />
        <InferenceAttemptDetail label={messages.modelLabel} code value={requestModel(request)} />
        <InferenceAttemptDetail
          label={messages.workingDirectoryLabel}
          code
          value={requestWorkingDirectory(request)}
        />
        <InferenceAttemptDetail label={messages.worktreeLabel} code value={requestWorktree(request)} />
        <InferenceAttemptDetail label={messages.outcomeLabel} value={scriptResponse.outcome} />
        <InferenceAttemptDetail
          label={messages.durationLabel}
          value={
            durationMillis !== undefined
              ? formatDurationMillis(durationMillis)
              : undefined
          }
        />
        <InferenceAttemptDetail
          label={messages.exitCodeLabel}
          value={exitCode !== undefined ? String(exitCode) : undefined}
        />
        <InferenceAttemptDetail label={messages.failureTypeLabel} code value={failureType} />
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
