import { formatDurationMillis } from "../../../../components/ui/formatters";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../../components/ui/widget-frame";
import type { WorkstationRequestDetailCardProps } from "../lib/detail-card-types";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
} from "../../components/current-selection-locale";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  REQUEST_HISTORY_TEXT_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
} from "../../base/components/detail-card-shared";
import type { WorkstationRequestDetailView } from "./workstation-request-detail-view";

export function ResponseDetailsSection({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  const messages = useCurrentSelectionDetailMessages();

  return (
    <section
      aria-label={messages.responseDetailsTitle}
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.responseDetailsTitle}</h4>
      {view.isScriptBackedRequest ? (
        <ScriptResponseDetails request={request} view={view} />
      ) : (
        <InferenceResponseDetails request={request} />
      )}
    </section>
  );
}

export function ErrorDetailsSection({
  view,
}: {
  view: WorkstationRequestDetailView;
}) {
  const messages = useCurrentSelectionDetailMessages();

  if (!view.hasFailureDetails) {
    return null;
  }

  return (
    <section
      aria-label={messages.errorDetailsTitle}
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.errorDetailsTitle}</h4>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <div>
          <dt>{messages.failureReasonLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedFailureReason ?? messages.failureReasonUnavailable}
          </dd>
        </div>
        <div>
          <dt>{messages.failureMessageLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.normalizedFailureMessage ?? messages.failureMessageUnavailable}
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
  const messages = useCurrentSelectionDetailMessages();
  const locale = useCurrentSelectionLocale();
  const scriptResponse = request.script_response;

  return (
    <>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <TraceIDField traceIDs={request.trace_ids} />
        {scriptResponse ? (
          <>
            <div>
              <dt>{messages.scriptRequestIdLabel}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.script_request_id ? (
                  <code className={RUNTIME_DETAIL_CODE_CLASS}>
                    {scriptResponse.script_request_id}
                  </code>
                ) : (
                  messages.scriptResponseUnavailableSummary
                )}
              </dd>
            </div>
            <div>
              <dt>{messages.scriptAttemptLabel}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.attempt ?? messages.scriptAttemptUnavailable}
              </dd>
            </div>
            <div>
              <dt>{messages.outcomeLabel}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.outcome ?? messages.outcomeUnavailable}
              </dd>
            </div>
            <div>
              <dt>{messages.durationLabel}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.duration_millis !== undefined
                  ? formatDurationMillis(scriptResponse.duration_millis, locale)
                  : messages.durationUnavailable}
              </dd>
            </div>
            <div>
              <dt>{messages.exitCodeLabel}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.exit_code ?? messages.exitCodeUnavailable}
              </dd>
            </div>
            <div>
              <dt>{messages.failureTypeLabel}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                {scriptResponse.failure_type ?? messages.failureTypeUnavailable}
              </dd>
            </div>
            <ScriptOutputField
              emptyMessage={messages.stdoutEmpty}
              title={messages.stdoutLabel}
              value={view.normalizedScriptStdout}
            />
            <ScriptOutputField
              emptyMessage={messages.stderrEmpty}
              title={messages.stderrLabel}
              value={view.normalizedScriptStderr}
            />
          </>
        ) : null}
      </dl>
      {request.script_response ? null : (
        <p className={DETAIL_COPY_CLASS}>
          {request.errored_request_count > 0 || view.hasFailureDetails
            ? messages.scriptResponseUnavailableErrored
            : messages.scriptResponseUnavailablePending}
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
  const messages = useCurrentSelectionDetailMessages();

  return (
    <div>
      <dt>{messages.traceIdsLabel}</dt>
      <dd className="grid gap-1">
        {traceIDs && traceIDs.length > 0 ? (
          traceIDs.map((traceId: string) => (
            <code className={RUNTIME_DETAIL_CODE_CLASS} key={traceId}>
              {traceId}
            </code>
          ))
        ) : (
          <span className={RUNTIME_DETAIL_VALUE_CLASS}>
            {messages.traceUnavailable}
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
          <pre className={REQUEST_HISTORY_TEXT_CLASS}>{value}</pre>
        ) : (
          emptyMessage
        )}
      </dd>
    </div>
  );
}
