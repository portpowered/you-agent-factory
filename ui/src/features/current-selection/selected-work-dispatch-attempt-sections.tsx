import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../components/dashboard/typography";
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

export function DispatchInferenceAttemptsSection({
  attempts,
}: {
  attempts: DashboardInferenceAttempt[];
}) {
  return (
    <section
      aria-label="Inference attempts"
      className="mt-[0.75rem] grid gap-[0.45rem] border-t border-af-overlay/8 pt-[0.75rem]"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Inference attempts</h4>
      <div className="grid gap-[0.65rem]">
        {attempts.map((attempt) => (
          <InferenceAttemptCard attempt={attempt} key={attempt.inference_request_id} />
        ))}
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
  return (
    <section
      aria-label="Script attempts"
      className="mt-[0.75rem] grid gap-[0.45rem] border-t border-af-overlay/8 pt-[0.75rem]"
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Script attempts</h4>
      <div className="grid gap-[0.65rem]">
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
          <p className={DETAIL_COPY_CLASS}>No script response attempt has been recorded yet.</p>
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
  const attemptNumber = scriptAttemptNumber(scriptRequest);
  const requestID = scriptRequestID(scriptRequest);

  return (
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-[0.8rem]">
        <div className="grid min-w-0 gap-[0.18rem]">
          <strong>Request attempt {attemptNumber ?? "pending"}</strong>
          <p className={`m-0 text-af-ink/70 ${DASHBOARD_BODY_TEXT_CLASS}`}>PENDING</p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>{requestID ?? "script-request"}</span>
      </div>
      <dl className={`mt-[0.65rem] ${INFERENCE_ATTEMPT_DETAIL_CLASS}`}>
        <InferenceAttemptDetail label="Script request ID" code value={requestID} />
        <InferenceAttemptDetail
          label="Script attempt"
          value={attemptNumber !== undefined ? String(attemptNumber) : undefined}
        />
        <InferenceAttemptDetail label="Provider" code value={requestProvider(request)} />
        <InferenceAttemptDetail label="Model" code value={requestModel(request)} />
        <InferenceAttemptDetail
          label="Working directory"
          code
          value={requestWorkingDirectory(request)}
        />
        <InferenceAttemptDetail label="Worktree" code value={requestWorktree(request)} />
        <InferenceAttemptDetail label="Command" code value={scriptRequest.command} />
      </dl>
      <ScriptArgsSection args={scriptRequest.args} />
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
  const attemptNumber = scriptAttemptNumber(scriptResponse) ?? fallbackAttemptNumber;
  const requestID = scriptRequestID(scriptResponse);
  const durationMillis = scriptResponseDurationMillis(scriptResponse);
  const exitCode = scriptResponseExitCode(scriptResponse);
  const failureType = scriptResponseFailureType(scriptResponse);

  return (
    <article className={PROVIDER_SESSION_CARD_CLASS}>
      <div className="flex items-start justify-between gap-[0.8rem]">
        <div className="grid min-w-0 gap-[0.18rem]">
          <strong>Response attempt {attemptNumber ?? "completed"}</strong>
          <p className={`m-0 text-af-ink/70 ${DASHBOARD_BODY_TEXT_CLASS}`}>
            {scriptResponse.outcome ?? "RECORDED"}
          </p>
        </div>
        <span className={EXECUTION_PILL_CLASS}>{requestID ?? "script-response"}</span>
      </div>
      <dl className={`mt-[0.65rem] ${INFERENCE_ATTEMPT_DETAIL_CLASS}`}>
        <InferenceAttemptDetail label="Script request ID" code value={requestID} />
        <InferenceAttemptDetail
          label="Script attempt"
          value={attemptNumber !== undefined ? String(attemptNumber) : undefined}
        />
        <InferenceAttemptDetail label="Provider" code value={requestProvider(request)} />
        <InferenceAttemptDetail label="Model" code value={requestModel(request)} />
        <InferenceAttemptDetail
          label="Working directory"
          code
          value={requestWorkingDirectory(request)}
        />
        <InferenceAttemptDetail label="Worktree" code value={requestWorktree(request)} />
        <InferenceAttemptDetail label="Outcome" value={scriptResponse.outcome} />
        <InferenceAttemptDetail
          label="Duration"
          value={
            durationMillis !== undefined
              ? formatDurationMillis(durationMillis)
              : undefined
          }
        />
        <InferenceAttemptDetail
          label="Exit code"
          value={exitCode !== undefined ? String(exitCode) : undefined}
        />
        <InferenceAttemptDetail label="Failure type" code value={failureType} />
      </dl>
      <ScriptOutputSection
        emptyMessage="No stdout was recorded for this script response."
        label="Stdout"
        value={normalizedStdout}
      />
      <ScriptOutputSection
        emptyMessage="No stderr was recorded for this script response."
        label="Stderr"
        value={normalizedStderr}
      />
    </article>
  );
}
