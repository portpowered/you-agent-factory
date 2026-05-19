import { formatDurationMillis } from "../../components/ui/formatters";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../components/dashboard/widget-board";
import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
} from "./detail-card-shared";
import type { WorkstationRequestDetailView } from "./workstation-request-detail-view";

const SCRIPT_OUTPUT_TEXT_CLASS =
  "m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 [overflow-wrap:anywhere]";

export function ResponseDetailsSection({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  return (
    <section
      aria-label="Response details"
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Response details</h4>
      {view.isScriptBackedRequest ? (
        <ScriptResponseDetails request={request} view={view} />
      ) : (
        <InferenceResponseDetails request={request} view={view} />
      )}
    </section>
  );
}

export function ErrorDetailsSection({
  view,
}: {
  view: WorkstationRequestDetailView;
}) {
  if (!view.hasFailureDetails) {
    return null;
  }

  return (
    <section
      aria-label="Error details"
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Error details</h4>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <div>
          <dt>Failure reason</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedFailureReason ??
              "Failure reason is not available for this request."}
          </dd>
        </div>
        <div>
          <dt>Failure message</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedFailureMessage ??
              "Failure message is not available for this request."}
          </dd>
        </div>
      </dl>
    </section>
  );
}

function ScriptResponseDetails({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  const scriptResponse = request.script_response;

  return (
    <>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <TraceIDField traceIDs={request.trace_ids} />
        {scriptResponse ? (
          <>
            <div>
              <dt>Script request ID</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.script_request_id ? (
                  <code className={RUNTIME_DETAIL_CODE_CLASS}>
                    {scriptResponse.script_request_id}
                  </code>
                ) : (
                  "Script response details are not available for this workstation request."
                )}
              </dd>
            </div>
            <div>
              <dt>Script attempt</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.attempt ??
                  "Script attempt is not available yet."}
              </dd>
            </div>
            <div>
              <dt>Outcome</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.outcome ??
                  "Outcome details are not available yet."}
              </dd>
            </div>
            <div>
              <dt>Duration</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.duration_millis !== undefined
                  ? formatDurationMillis(scriptResponse.duration_millis)
                  : "Duration details are not available for this script response yet."}
              </dd>
            </div>
            <div>
              <dt>Exit code</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.exit_code ??
                  "Exit code is not available for this script response."}
              </dd>
            </div>
            <div>
              <dt>Failure type</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.failure_type ??
                  "Failure type is not available for this script response."}
              </dd>
            </div>
            <ScriptOutputField
              emptyMessage="No stdout was recorded for this script response."
              title="Stdout"
              value={view.normalizedScriptStdout}
            />
            <ScriptOutputField
              emptyMessage="No stderr was recorded for this script response."
              title="Stderr"
              value={view.normalizedScriptStderr}
            />
          </>
        ) : null}
      </dl>
      {request.script_response ? null : (
        <p className={DETAIL_COPY_CLASS}>
          {view.scriptResponseUnavailableCopy}
        </p>
      )}
    </>
  );
}

function InferenceResponseDetails({
  request,
}: {
  request: WorkstationRequestDetailCardProps["request"];
}) {
  return (
    <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
      <TraceIDField traceIDs={request.trace_ids} />
    </dl>
  );
}

function TraceIDField({
  traceIDs,
}: {
  traceIDs: WorkstationRequestDetailCardProps["request"]["trace_ids"];
}) {
  return (
    <div>
      <dt>Trace IDs</dt>
      <dd className="grid gap-1">
        {traceIDs && traceIDs.length > 0 ? (
          traceIDs.map((traceId: string) => (
            <code className={RUNTIME_DETAIL_CODE_CLASS} key={traceId}>
              {traceId}
            </code>
          ))
        ) : (
          <span className={RUNTIME_DETAIL_VALUE_CLASS}>
            Trace details are not available for this workstation request yet.
          </span>
        )}
      </dd>
    </div>
  );
}

function ScriptOutputField({
  emptyMessage,
  title,
  value,
}: {
  emptyMessage: string;
  title: string;
  value: string | undefined;
}) {
  return (
    <div>
      <dt>{title}</dt>
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {value ? (
          <pre className={SCRIPT_OUTPUT_TEXT_CLASS}>{value}</pre>
        ) : (
          emptyMessage
        )}
      </dd>
    </div>
  );
}
