import {
  formatDurationFromISO,
  formatDurationMillis,
} from "../../components/ui/formatters";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  InferenceAttemptDetail,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
  TRACE_ACTION_LINK_CLASS,
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

  return (
    <section aria-label="Execution details" className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Execution details</h4>
      <dl>
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
          label="startedAt"
          value={request.startedAt ?? request.started_at}
        />
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
      <p className={DETAIL_COPY_CLASS}>
        Prompt, provider-session, and response-body details are shown under Inference attempts.
      </p>
    </section>
  );
}
