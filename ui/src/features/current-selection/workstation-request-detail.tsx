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
  const isScriptBackedRequest =
    request.script_request !== undefined || request.script_response !== undefined;
  const normalizedPrompt = normalizeDetailText(request.prompt);
  const normalizedResponse = normalizeDetailText(request.response);
  const normalizedFailureReason = normalizeDetailText(request.failure_reason);
  const normalizedFailureMessage = normalizeDetailText(request.failure_message);
  const normalizedScriptStdout = normalizeDetailText(request.script_response?.stdout);
  const normalizedScriptStderr = normalizeDetailText(request.script_response?.stderr);
  const hasFailureDetails =
    normalizedFailureReason !== undefined || normalizedFailureMessage !== undefined;
  const hasErroredRequest = request.errored_request_count > 0 || hasFailureDetails;
  const providerUnavailableCopy = isScriptBackedRequest
    ? "Provider details are not applicable to this script-backed workstation request."
    : "Provider details are not available for this workstation request.";
  const modelUnavailableCopy = isScriptBackedRequest
    ? "Model details are not applicable to this script-backed workstation request."
    : "Model details are not available for this workstation request.";
  const outcome = request.outcome ?? request.script_response?.outcome;
  const totalDurationMillis =
    request.total_duration_millis ?? request.script_response?.duration_millis;
  const requestMetadata = request.request_metadata;
  const scriptRequest = request.script_request;
  const scriptResponse = request.script_response;
  const traceIDs = request.trace_ids;
  const responseUnavailableCopy = hasErroredRequest
    ? "Response text is unavailable because this workstation request ended with an error."
    : "Response text is not available for this workstation request yet.";
  const responseMetadataUnavailableCopy = hasErroredRequest
    ? "Response metadata is unavailable because this workstation request ended with an error."
    : isScriptBackedRequest
      ? "Response metadata is not available for this script-backed workstation request."
      : "Response metadata is not available for this workstation request yet.";
  const scriptResponseUnavailableCopy = hasErroredRequest
    ? "Script response details are unavailable because this workstation request ended with an error."
    : "Script response details are not available for this workstation request yet.";

  return (
    <SelectionDetailLayout widgetId={widgetId}>
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
              providerUnavailableCopy
            )}
          </dd>
        </div>
        <div>
          <dt>Model</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.model ? (
              <code className={RUNTIME_DETAIL_CODE_CLASS}>{request.model}</code>
            ) : (
              modelUnavailableCopy
            )}
          </dd>
        </div>
        <div>
          <dt>Outcome</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {outcome ? outcome : "Outcome details are not available yet."}
          </dd>
        </div>
        <div>
          <dt>Total duration</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {totalDurationMillis !== undefined
              ? formatDurationMillis(totalDurationMillis)
              : "Total duration is not available for this workstation request yet."}
          </dd>
        </div>
      </dl>
      <RequestCountSection request={request} />
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
              {normalizedPrompt ? (
                <RequestAuthoredText value={normalizedPrompt} />
              ) : scriptRequest ? (
                "Prompt details are not applicable to this script-backed workstation request."
              ) : (
                "Prompt details are not available for this workstation request yet."
              )}
            </dd>
          </div>
          {scriptRequest ? (
            <>
              <div>
                <dt>Script request ID</dt>
                <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                  {scriptRequest.script_request_id ? (
                    <code className={RUNTIME_DETAIL_CODE_CLASS}>
                      {scriptRequest.script_request_id}
                    </code>
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
                    <code className={RUNTIME_DETAIL_CODE_CLASS}>
                      {scriptRequest.command}
                    </code>
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
          ) : null}
        </dl>
      </section>
      <MetadataSection
        emptyMessage="Request metadata is not available for this workstation request."
        metadata={requestMetadata}
        title="Request metadata"
      />
      <section aria-label="Response details" className={RUNTIME_DETAILS_SECTION_CLASS}>
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Response details</h4>
        {isScriptBackedRequest ? (
          <>
            <dl>
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
                        ? formatDurationMillis(
                            scriptResponse.duration_millis,
                          )
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
                  <div>
                    <dt>Stdout</dt>
                    <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                      {normalizedScriptStdout ? (
                        <pre className={SCRIPT_OUTPUT_TEXT_CLASS}>{normalizedScriptStdout}</pre>
                      ) : (
                        "No stdout was recorded for this script response."
                      )}
                    </dd>
                  </div>
                  <div>
                    <dt>Stderr</dt>
                    <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                      {normalizedScriptStderr ? (
                        <pre className={SCRIPT_OUTPUT_TEXT_CLASS}>{normalizedScriptStderr}</pre>
                      ) : (
                        "No stderr was recorded for this script response."
                      )}
                    </dd>
                  </div>
                </>
              ) : null}
            </dl>
            {request.script_response ? null : (
              <p className={DETAIL_COPY_CLASS}>{scriptResponseUnavailableCopy}</p>
            )}
          </>
        ) : (
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
            <div>
              <dt>Trace IDs</dt>
              <dd className="grid gap-[0.25rem]">
                {request.trace_ids && request.trace_ids.length > 0 ? (
                  request.trace_ids.map((traceId) => (
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
            <div>
              <dt>{WORKSTATION_RESPONSE_TEXT_LABEL}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {normalizedResponse ? (
                  <pre className={INFERENCE_ATTEMPT_TEXT_CLASS}>{normalizedResponse}</pre>
                ) : (
                  responseUnavailableCopy
                )}
              </dd>
            </div>
          </dl>
        )}
      </section>
      <MetadataSection
        emptyMessage={responseMetadataUnavailableCopy}
        metadata={request.response_metadata}
        title="Response metadata"
      />
      {hasFailureDetails ? (
        <section aria-label="Error details" className={RUNTIME_DETAILS_SECTION_CLASS}>
          <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Error details</h4>
          <dl>
            <div>
              <dt>Failure reason</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {normalizedFailureReason ?? "Failure reason is not available for this request."}
              </dd>
            </div>
            <div>
              <dt>Failure message</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {normalizedFailureMessage ?? "Failure message is not available for this request."}
              </dd>
            </div>
          </dl>
        </section>
      ) : null}
      {request.inference_attempts.length > 0 ? (
        <InferenceAttemptsSection attempts={request.inference_attempts} />
      ) : null}
    </SelectionDetailLayout>
  );
}
