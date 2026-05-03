import { formatDurationMillis, formatProviderSession } from "../../components/ui/formatters";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS, WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  INFERENCE_ATTEMPT_TEXT_CLASS,
  MetadataSection,
  normalizeDetailText,
  RequestAuthoredText,
  RequestCountSection,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
  WORKSTATION_RESPONSE_TEXT_LABEL,
} from "./detail-card-shared";
import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { InferenceAttemptsSection } from "./execution-details";
const SCRIPT_OUTPUT_TEXT_CLASS =
  "m-0 whitespace-pre-wrap rounded-lg border border-af-overlay/8 bg-af-overlay/6 p-2 [overflow-wrap:anywhere]";

export function WorkstationRequestDetailCard({
  request,
  widgetId = "current-selection",
}: WorkstationRequestDetailCardProps) {
  const view = buildWorkstationRequestDetailView(request);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <WorkstationRequestSummary request={request} view={view} />
      <RequestCountSection request={request} />
      <RequestDetailsSection request={request} view={view} />
      <MetadataSection
        emptyMessage="Request metadata is not available for this workstation request."
        metadata={request.request_metadata}
        title="Request metadata"
      />
      <ResponseDetailsSection request={request} view={view} />
      <MetadataSection
        emptyMessage={view.responseMetadataUnavailableCopy}
        metadata={request.response_metadata}
        title="Response metadata"
      />
      <ErrorDetailsSection view={view} />
      {request.inference_attempts.length > 0 ? (
        <InferenceAttemptsSection attempts={request.inference_attempts} />
      ) : null}
    </SelectionDetailLayout>
  );
}

interface WorkstationRequestDetailView {
  hasFailureDetails: boolean;
  isScriptBackedRequest: boolean;
  modelUnavailableCopy: string;
  normalizedFailureMessage: string | undefined;
  normalizedFailureReason: string | undefined;
  normalizedPrompt: string | undefined;
  normalizedResponse: string | undefined;
  normalizedScriptStderr: string | undefined;
  normalizedScriptStdout: string | undefined;
  outcome: string | undefined;
  providerUnavailableCopy: string;
  responseMetadataUnavailableCopy: string;
  responseUnavailableCopy: string;
  scriptResponseUnavailableCopy: string;
  totalDurationMillis: number | undefined;
}

function buildWorkstationRequestDetailView(
  request: WorkstationRequestDetailCardProps["request"],
): WorkstationRequestDetailView {
  const isScriptBackedRequest =
    request.script_request !== undefined || request.script_response !== undefined;
  const normalizedFailureReason = normalizeDetailText(request.failure_reason);
  const normalizedFailureMessage = normalizeDetailText(request.failure_message);
  const hasFailureDetails =
    normalizedFailureReason !== undefined || normalizedFailureMessage !== undefined;
  const hasErroredRequest = request.errored_request_count > 0 || hasFailureDetails;

  return {
    hasFailureDetails,
    isScriptBackedRequest,
    modelUnavailableCopy: isScriptBackedRequest
      ? "Model details are not applicable to this script-backed workstation request."
      : "Model details are not available for this workstation request.",
    normalizedFailureMessage,
    normalizedFailureReason,
    normalizedPrompt: normalizeDetailText(request.prompt),
    normalizedResponse: normalizeDetailText(request.response),
    normalizedScriptStderr: normalizeDetailText(request.script_response?.stderr),
    normalizedScriptStdout: normalizeDetailText(request.script_response?.stdout),
    outcome: request.outcome ?? request.script_response?.outcome,
    providerUnavailableCopy: isScriptBackedRequest
      ? "Provider details are not applicable to this script-backed workstation request."
      : "Provider details are not available for this workstation request.",
    responseMetadataUnavailableCopy: hasErroredRequest
      ? "Response metadata is unavailable because this workstation request ended with an error."
      : isScriptBackedRequest
        ? "Response metadata is not available for this script-backed workstation request."
        : "Response metadata is not available for this workstation request yet.",
    responseUnavailableCopy: hasErroredRequest
      ? "Response text is unavailable because this workstation request ended with an error."
      : "Response text is not available for this workstation request yet.",
    scriptResponseUnavailableCopy: hasErroredRequest
      ? "Script response details are unavailable because this workstation request ended with an error."
      : "Script response details are not available for this workstation request yet.",
    totalDurationMillis:
      request.total_duration_millis ?? request.script_response?.duration_millis,
  };
}

function WorkstationRequestSummary({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  return (
    <>
      <p className={WIDGET_SUBTITLE_CLASS}>{request.request_id || request.dispatch_id}</p>
      <dl>
        <div>
          <dt>Dispatch ID</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.dispatch_id}</code>
          </dd>
        </div>
        <div>
          <dt>Request ID</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.request_id ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.request_id}</code>
            ) : (
              "Request ID is not available for this workstation request."
            )}
          </dd>
        </div>
        <div>
          <dt>Workstation</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.workstation_name || "Workstation details are not available for this request."}
          </dd>
        </div>
        <div>
          <dt>Transition ID</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.transition_id}</code>
          </dd>
        </div>
        <div>
          <dt>Provider</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.provider ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.provider}</code>
            ) : (
              view.providerUnavailableCopy
            )}
          </dd>
        </div>
        <div>
          <dt>Model</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.model ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.model}</code>
            ) : (
              view.modelUnavailableCopy
            )}
          </dd>
        </div>
        <div>
          <dt>Outcome</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.outcome ? view.outcome : "Outcome details are not available yet."}
          </dd>
        </div>
        <div>
          <dt>Total duration</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.totalDurationMillis !== undefined
              ? formatDurationMillis(view.totalDurationMillis)
              : "Total duration is not available for this workstation request yet."}
          </dd>
        </div>
      </dl>
    </>
  );
}

function RequestDetailsSection({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  return (
    <section aria-label="Request details" className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Request details</h4>
      <dl>
        <div>
          <dt>Working directory</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.working_directory ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.working_directory}</code>
            ) : (
              "Working directory details are not available for this workstation request."
            )}
          </dd>
        </div>
        <div>
          <dt>Worktree</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.worktree ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.worktree}</code>
            ) : (
              "Worktree details are not available for this workstation request."
            )}
          </dd>
        </div>
        <div>
          <dt>Prompt</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedPrompt ? (
              <RequestAuthoredText value={view.normalizedPrompt} />
            ) : request.script_request ? (
              "Prompt details are not applicable to this script-backed workstation request."
            ) : (
              "Prompt details are not available for this workstation request yet."
            )}
          </dd>
        </div>
        <ScriptRequestFields request={request} />
      </dl>
    </section>
  );
}

function ScriptRequestFields({
  request,
}: {
  request: WorkstationRequestDetailCardProps["request"];
}) {
  const scriptRequest = request.script_request;
  if (!scriptRequest) {
    return null;
  }

  return (
    <>
      <div>
        <dt>Script request ID</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {scriptRequest.script_request_id ? (
            <code className={RUNTIME_DETAIL_CODE_CLASS}>{scriptRequest.script_request_id}</code>
          ) : (
            "Script request details are not available for this workstation request."
          )}
        </dd>
      </div>
      <div>
        <dt>Script attempt</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {scriptRequest.attempt ?? "Script attempt is not available yet."}
        </dd>
      </div>
      <div>
        <dt>Command</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {scriptRequest.command ? (
            <code className={RUNTIME_DETAIL_CODE_CLASS}>{scriptRequest.command}</code>
          ) : (
            "Script command details are not available for this workstation request."
          )}
        </dd>
      </div>
      <div>
        <dt>Resolved args</dt>
        <dd className="grid gap-[0.25rem]">
          {scriptRequest.args && scriptRequest.args.length > 0 ? (
            scriptRequest.args.map((arg: string) => (
              <code className={RUNTIME_DETAIL_CODE_CLASS} key={arg}>
                {arg}
              </code>
            ))
          ) : (
            <span className={RUNTIME_DETAIL_VALUE_CLASS}>
              Script arguments are not available for this workstation request.
            </span>
          )}
        </dd>
      </div>
    </>
  );
}

function ResponseDetailsSection({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  return (
    <section aria-label="Response details" className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Response details</h4>
      {view.isScriptBackedRequest ? (
        <ScriptResponseDetails request={request} view={view} />
      ) : (
        <InferenceResponseDetails request={request} view={view} />
      )}
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
      <dl>
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
                {scriptResponse.attempt ?? "Script attempt is not available yet."}
              </dd>
            </div>
            <div>
              <dt>Outcome</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.outcome ?? "Outcome details are not available yet."}
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
                {scriptResponse.exit_code ?? "Exit code is not available for this script response."}
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
        <p className={DETAIL_COPY_CLASS}>{view.scriptResponseUnavailableCopy}</p>
      )}
    </>
  );
}

function InferenceResponseDetails({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  return (
    <dl>
      <div>
        <dt>Provider session</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {request.provider_session?.id ? (
            <code className={RUNTIME_DETAIL_CODE_CLASS}>
              {formatProviderSession(request.provider_session)}
            </code>
          ) : (
            "Provider session details are not available for this workstation request."
          )}
        </dd>
      </div>
      <TraceIDField traceIDs={request.trace_ids} />
      <div>
        <dt>{WORKSTATION_RESPONSE_TEXT_LABEL}</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {view.normalizedResponse ? (
            <pre className={INFERENCE_ATTEMPT_TEXT_CLASS}>{view.normalizedResponse}</pre>
          ) : (
            view.responseUnavailableCopy
          )}
        </dd>
      </div>
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
      <dd className="grid gap-[0.25rem]">
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
        {value ? <pre className={SCRIPT_OUTPUT_TEXT_CLASS}>{value}</pre> : emptyMessage}
      </dd>
    </div>
  );
}

function ErrorDetailsSection({
  view,
}: {
  view: WorkstationRequestDetailView;
}) {
  if (!view.hasFailureDetails) {
    return null;
  }

  return (
    <section aria-label="Error details" className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Error details</h4>
      <dl>
        <div>
          <dt>Failure reason</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedFailureReason ?? "Failure reason is not available for this request."}
          </dd>
        </div>
        <div>
          <dt>Failure message</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedFailureMessage ?? "Failure message is not available for this request."}
          </dd>
        </div>
      </dl>
    </section>
  );
}
