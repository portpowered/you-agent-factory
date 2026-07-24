import { CodePanel } from "@you-agent-factory/components/data-display";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { formatDurationMillis } from "../../../../../components/ui/formatters";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailCode,
  CurrentSelectionDetailItem,
  CurrentSelectionDetailValue,
  CurrentSelectionExpandableSection,
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
} from "../../../base/public";
import type { WorkstationRequestDetailCardProps } from "../../lib/detail-card-types";
import type { WorkstationRequestDetailView } from "../request-detail/workstation-request-detail-view";

export function ResponseDetailsSection({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  const messages = useCurrentSelectionDetailMessages();

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      title={messages.responseDetailsTitle}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {view.isScriptBackedRequest ? (
        <ScriptResponseDetails request={request} view={view} />
      ) : (
        <InferenceResponseDetails request={request} />
      )}
    </CurrentSelectionExpandableSection>
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
    <CurrentSelectionExpandableSection
      defaultExpanded
      title={messages.errorDetailsTitle}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <CurrentSelectionDescriptionList>
        <CurrentSelectionDetailItem
          label={messages.failureReasonLabel}
          value={
            view.normalizedFailureReason ?? messages.failureReasonUnavailable
          }
        />
        <CurrentSelectionDetailItem
          label={messages.failureMessageLabel}
          value={
            view.normalizedFailureMessage ?? messages.failureMessageUnavailable
          }
        />
      </CurrentSelectionDescriptionList>
    </CurrentSelectionExpandableSection>
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
      <CurrentSelectionDescriptionList>
        <TraceIDField traceIDs={request.trace_ids} />
        {scriptResponse ? (
          <>
            <CurrentSelectionDetailItem
              code={Boolean(scriptResponse.script_request_id)}
              label={messages.scriptRequestIdLabel}
              value={
                scriptResponse.script_request_id ??
                messages.scriptResponseUnavailableSummary
              }
            />
            <CurrentSelectionDetailItem
              label={messages.scriptAttemptLabel}
              value={
                scriptResponse.attempt ?? messages.scriptAttemptUnavailable
              }
            />
            <CurrentSelectionDetailItem
              label={messages.outcomeLabel}
              value={scriptResponse.outcome ?? messages.outcomeUnavailable}
            />
            <CurrentSelectionDetailItem
              label={messages.durationLabel}
              value={
                scriptResponse.duration_millis !== undefined
                  ? formatDurationMillis(scriptResponse.duration_millis, locale)
                  : messages.durationUnavailable
              }
            />
            <CurrentSelectionDetailItem
              label={messages.exitCodeLabel}
              value={scriptResponse.exit_code ?? messages.exitCodeUnavailable}
            />
            <CurrentSelectionDetailItem
              label={messages.failureTypeLabel}
              value={
                scriptResponse.failure_type ?? messages.failureTypeUnavailable
              }
            />
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
      </CurrentSelectionDescriptionList>
      {request.script_response ? null : (
        <WidgetDetailCopy>
          {request.errored_request_count > 0 || view.hasFailureDetails
            ? messages.scriptResponseUnavailableErrored
            : messages.scriptResponseUnavailablePending}
        </WidgetDetailCopy>
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
    <CurrentSelectionDescriptionList>
      <TraceIDField traceIDs={request.trace_ids} />
    </CurrentSelectionDescriptionList>
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
            <CurrentSelectionDetailCode key={traceId}>
              {traceId}
            </CurrentSelectionDetailCode>
          ))
        ) : (
          <span className="min-w-0 [overflow-wrap:anywhere]">
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
      <CurrentSelectionDetailValue>
        {value ? <CodePanel>{value}</CodePanel> : emptyMessage}
      </CurrentSelectionDetailValue>
    </div>
  );
}
