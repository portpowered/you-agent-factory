import {
  formatDurationFromISO,
  formatDurationMillis,
  getProviderSessionLogTarget,
} from "../../components/ui/formatters";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  INFERENCE_ATTEMPT_TEXT_CLASS,
  INFERENCE_REQUEST_PROMPT_LABEL,
  InferenceAttemptDetail,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
  TRACE_ACTION_LINK_CLASS,
  WORKSTATION_RESPONSE_TEXT_LABEL,
  formatExecutionDetailValue,
} from "./detail-card-shared";
import type {
  ExecutionDetailsSectionProps,
  InferenceAttemptsSectionProps,
} from "./detail-card-types";
import { InferenceAttemptCard } from "./inference-attempt";

export function ExecutionDetailsSection({
  activeTraceID,
  details,
  now,
  onSelectTraceID,
  showInferenceAttempts = true,
  traceTargetId,
}: ExecutionDetailsSectionProps) {
  const hasTraceIDs = details.traceIDs.length > 0;
  const providerSessionLogTarget = getProviderSessionLogTarget(
    details.providerSessionData,
    details.elapsedStartTimestamp,
  );

  return (
    <section aria-label="Execution details" className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Execution details</h4>
      <dl>
        <div>
          <dt>Provider</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {formatExecutionDetailValue(details.provider, "Provider")}
          </dd>
        </div>
        {details.model.status !== "omitted" ? (
          <div>
            <dt>Model</dt>
            <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
              {formatExecutionDetailValue(details.model, "Model")}
            </dd>
          </div>
        ) : null}
        <PromptDiagnosticDetail prompt={details.prompt} />
        <div>
          <dt>Provider session</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.providerSession.status === "available" ? (
              providerSessionLogTarget ? (
                <a
                  className={TRACE_ACTION_LINK_CLASS}
                  href={providerSessionLogTarget.href}
                  title={providerSessionLogTarget.display}
                >
                  {details.providerSession.value}
                </a>
              ) : (
                <code className={RUNTIME_DETAIL_CODE_CLASS}>{details.providerSession.value}</code>
              )
            ) : (
              formatExecutionDetailValue(details.providerSession, "Provider session")
            )}
          </dd>
        </div>
        <div>
          <dt>Dispatch ID</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.dispatchID ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{details.dispatchID}</code>
            ) : (
              "Dispatch ID is not available for this selected run."
            )}
          </dd>
        </div>
        <div>
          <dt>Workstation</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.workstationName ||
              "Workstation details are not available for this selected run."}
          </dd>
        </div>
        <div>
          <dt>Elapsed</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {details.elapsedStartTimestamp
              ? formatDurationFromISO(details.elapsedStartTimestamp, now)
              : "Elapsed time is not available for this selected run."}
          </dd>
        </div>
        <div>
          <dt>Trace IDs</dt>
          <dd className="grid gap-[0.35rem]">
            {hasTraceIDs ? (
              details.traceIDs.map((traceID) => (
                  <a
                    className={TRACE_ACTION_LINK_CLASS}
                    href={`#${traceTargetId}`}
                    key={traceID}
                    onClick={() => onSelectTraceID?.(traceID)}
                  >
                    {traceID}
                    {activeTraceID === traceID ? " (selected)" : ""}
                  </a>
                ))
            ) : (
              <span className={RUNTIME_DETAIL_VALUE_CLASS}>
                Trace details are not available for this selected run.
              </span>
            )}
          </dd>
        </div>
      </dl>
      {hasTraceIDs ? (
        <div className="grid gap-[0.55rem]">
          <p className={DETAIL_COPY_CLASS}>
            Open the trace to review dispatches, retries, and workstation output for this work
            item.
          </p>
          <a
            className={TRACE_ACTION_LINK_CLASS}
            href={`#${traceTargetId}`}
            onClick={() => onSelectTraceID?.(activeTraceID ?? details.traceIDs[0] ?? "")}
          >
            Open trace
          </a>
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>Trace details are not available for this selected run.</p>
      )}
      <WorkstationRequestProjectionSection details={details} />
      {showInferenceAttempts ? <InferenceAttemptsSection attempts={details.inferenceAttempts} /> : null}
    </section>
  );
}

export function InferenceAttemptsSection({ attempts }: InferenceAttemptsSectionProps) {
  return (
    <section aria-label="Inference attempts" className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Inference attempts</h4>
      {attempts.length > 0 ? (
        <div className="grid gap-[0.8rem]">
          {attempts.map((attempt) => (
            <InferenceAttemptCard attempt={attempt} key={attempt.inference_request_id} />
          ))}
        </div>
      ) : (
        <p className={DETAIL_COPY_CLASS}>
          No inference events are available for this selected work item.
        </p>
      )}
    </section>
  );
}

function PromptDiagnosticDetail({ prompt }: { prompt: ExecutionDetailsSectionProps["details"]["prompt"] }) {
  if (prompt.status === "available") {
    return (
      <div>
        <dt>Prompt</dt>
        <dd className="grid gap-[0.25rem]">
          {prompt.promptSource ? (
            <span className={RUNTIME_DETAIL_VALUE_CLASS}>
              Source: <code className={RUNTIME_DETAIL_CODE_CLASS}>{prompt.promptSource}</code>
            </span>
          ) : null}
          {prompt.systemPromptHash ? (
            <span className={RUNTIME_DETAIL_VALUE_CLASS}>
              System prompt hash:{" "}
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{prompt.systemPromptHash}</code>
            </span>
          ) : null}
        </dd>
      </div>
    );
  }

  return (
    <div>
      <dt>Prompt</dt>
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {prompt.status === "pending"
          ? "Prompt details are not available for this selected run yet."
          : "Prompt details are not available for this selected run."}
      </dd>
    </div>
  );
}

function WorkstationRequestProjectionSection({
  details,
}: Pick<ExecutionDetailsSectionProps, "details">) {
  const requestProjection = details.workstationRequest;
  if (!requestProjection) {
    return null;
  }

  const { counts, request, response } = requestProjection;

  return (
    <section aria-label="Workstation request" className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Workstation request</h4>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <InferenceAttemptDetail
          label="dispatchedCount"
          value={counts.dispatchedCount ?? counts.dispatched_count}
        />
        <InferenceAttemptDetail
          label="respondedCount"
          value={counts.respondedCount ?? counts.responded_count}
        />
        <InferenceAttemptDetail
          label="erroredCount"
          value={counts.erroredCount ?? counts.errored_count}
        />
        <InferenceAttemptDetail
          code
          label="requestTime"
          value={request.requestTime ?? request.request_time}
        />
        <InferenceAttemptDetail code label="startedAt" value={request.startedAt ?? request.started_at} />
        <InferenceAttemptDetail
          code
          label="workingDirectory"
          value={request.workingDirectory ?? request.working_directory}
        />
        <InferenceAttemptDetail code label="worktree" value={request.worktree} />
        <InferenceAttemptDetail code label="outcome" value={response?.outcome} />
        <InferenceAttemptDetail
          label="duration"
          value={
            (response?.durationMillis ?? response?.duration_millis) !== undefined
              ? formatDurationMillis(
                  response?.durationMillis ?? response?.duration_millis ?? 0,
                )
              : undefined
          }
        />
        <InferenceAttemptDetail code label="errorClass" value={response?.errorClass ?? response?.error_class} />
        <InferenceAttemptDetail
          code
          label="failureReason"
          value={response?.failureReason ?? response?.failure_reason}
        />
        <InferenceAttemptDetail
          code
          label="failureMessage"
          value={response?.failureMessage ?? response?.failure_message}
        />
      </dl>
      {request.prompt ? (
        <div className="grid gap-[0.3rem]">
          <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {INFERENCE_REQUEST_PROMPT_LABEL}
          </span>
          <pre className={INFERENCE_ATTEMPT_TEXT_CLASS}>{request.prompt}</pre>
        </div>
      ) : null}
      {response?.responseText ?? response?.response_text ? (
        <div className="grid gap-[0.3rem]">
          <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {WORKSTATION_RESPONSE_TEXT_LABEL}
          </span>
          <pre className={INFERENCE_ATTEMPT_TEXT_CLASS}>
            {response?.responseText ?? response?.response_text}
          </pre>
        </div>
      ) : response ? (
        <p className={DETAIL_COPY_CLASS}>
          Provider response text is not available on the workstation request projection.
        </p>
      ) : (
        <p className={DETAIL_COPY_CLASS}>
          The workstation request has not produced a response yet.
        </p>
      )}
    </section>
  );
}
