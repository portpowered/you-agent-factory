import { cn } from "../../lib/cn";
import { formatWorkItemLabel } from "../../components/ui/formatters";
import { formatDurationMillis } from "../../components/ui/formatters";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  MetadataSection,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { InferenceAttemptsSection } from "./execution-details";
import {
  ErrorDetailsSection,
  ResponseDetailsSection,
} from "./workstation-request-detail-response";
import {
  buildWorkstationRequestDetailView,
  type WorkstationRequestDetailView,
} from "./workstation-request-detail-view";
import { useCurrentSelectionDetailMessages } from "./current-selection-locale";

export function WorkstationRequestDetailCard({
  onSelectWorkID,
  request,
  selectedWorkID,
  widgetId = "current-selection",
}: WorkstationRequestDetailCardProps) {
  const messages = useCurrentSelectionDetailMessages();
  const view = buildWorkstationRequestDetailView(request);
  const showInferenceAttempts = !view.isScriptBackedRequest;

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <WorkstationRequestSummary request={request} view={view} />
      <RequestDetailsSection
        onSelectWorkID={onSelectWorkID}
        request={request}
        selectedWorkID={selectedWorkID}
        view={view}
      />
      {view.isScriptBackedRequest ? (
        <MetadataSection
          emptyMessage={messages.metadataEmpty}
          metadata={request.request_metadata}
          title={messages.requestMetadataTitle}
        />
      ) : null}
      <ResponseDetailsSection request={request} view={view} />
      {view.isScriptBackedRequest ? (
        <MetadataSection
          emptyMessage={
            request.errored_request_count > 0 || view.hasFailureDetails
              ? messages.responseMetadataUnavailableErrored
              : messages.responseMetadataUnavailableScript
          }
          metadata={request.response_metadata}
          title={messages.responseMetadataTitle}
        />
      ) : null}
      <ErrorDetailsSection view={view} />
      {showInferenceAttempts ? (
        <InferenceAttemptsSection attempts={request.inference_attempts} />
      ) : null}
    </SelectionDetailLayout>
  );
}

function WorkstationRequestSummary({
  request,
  view,
}: {
  request: WorkstationRequestDetailCardProps["request"];
  view: WorkstationRequestDetailView;
}) {
  const messages = useCurrentSelectionDetailMessages();

  return (
    <>
      <p className={WIDGET_SUBTITLE_CLASS}>
        {request.request_id || request.dispatch_id}
      </p>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <div>
          <dt>{messages.dispatchIdLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            <code className={RUNTIME_DETAIL_CODE_CLASS}>
              {request.dispatch_id}
            </code>
          </dd>
        </div>
        {request.request_id ? (
          <div>
            <dt>{messages.requestIdLabel}</dt>
            <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
              <code className={RUNTIME_DETAIL_CODE_CLASS}>
                {request.request_id}
              </code>
            </dd>
          </div>
        ) : null}
        <div>
          <dt>{messages.workstationLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.workstation_name || messages.workstationUnavailable}
          </dd>
        </div>
        <div>
          <dt>{messages.outcomeLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.outcome ? (
              <span className="flex flex-wrap gap-x-2 gap-y-1">
                <span>{view.outcome}</span>
                {view.hasFailedOutcome && view.normalizedFailureReason ? (
                  <span>
                    {messages.failureReasonLabel}: {view.normalizedFailureReason}
                  </span>
                ) : null}
                {view.hasFailedOutcome && view.normalizedFailureMessage ? (
                  <span>
                    {messages.failureMessageLabel}:{" "}
                    {view.normalizedFailureMessage}
                  </span>
                ) : null}
              </span>
            ) : (
              messages.outcomeUnavailable
            )}
          </dd>
        </div>
        <div>
          <dt>{messages.totalDurationLabel}</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {view.totalDurationMillis !== undefined
              ? formatDurationMillis(view.totalDurationMillis)
              : messages.totalDurationUnavailable}
          </dd>
        </div>
      </dl>
    </>
  );
}

function RequestDetailsSection({
  onSelectWorkID,
  request,
  selectedWorkID,
  view,
}: {
  onSelectWorkID?: WorkstationRequestDetailCardProps["onSelectWorkID"];
  request: WorkstationRequestDetailCardProps["request"];
  selectedWorkID?: WorkstationRequestDetailCardProps["selectedWorkID"];
  view: WorkstationRequestDetailView;
}) {
  const messages = useCurrentSelectionDetailMessages();
  const consumedWorkItems = request.request_view?.input_work_items ?? [];

  if (!view.isScriptBackedRequest) {
    return (
      <section
        aria-label={messages.requestDetailsTitle}
        className={RUNTIME_DETAILS_SECTION_CLASS}
      >
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.requestDetailsTitle}</h4>
        <ConsumedWorkItemsSection
          onSelectWorkID={onSelectWorkID}
          selectedWorkID={selectedWorkID}
          workItems={consumedWorkItems}
        />
      </section>
    );
  }

  return (
    <section
      aria-label={messages.requestDetailsTitle}
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{messages.requestDetailsTitle}</h4>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <ScriptRequestFields request={request} />
      </dl>
      <ConsumedWorkItemsSection
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        workItems={consumedWorkItems}
      />
    </section>
  );
}

function ConsumedWorkItemsSection({
  onSelectWorkID,
  selectedWorkID,
  workItems,
}: {
  onSelectWorkID?: WorkstationRequestDetailCardProps["onSelectWorkID"];
  selectedWorkID?: WorkstationRequestDetailCardProps["selectedWorkID"];
  workItems: NonNullable<
    NonNullable<
      WorkstationRequestDetailCardProps["request"]["request_view"]
    >["input_work_items"]
  >;
}) {
  const messages = useCurrentSelectionDetailMessages();

  if (workItems.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <span>{messages.consumedWorkItemsLabel}</span>
      <div className="flex flex-wrap gap-2">
        {workItems.map((workItem) => {
          const workLabel = formatWorkItemLabel(workItem);
          const isSelected = selectedWorkID === workItem.work_id;

          return (
            <button
              aria-label={messages.selectWorkItemLabel(workLabel)}
              aria-pressed={isSelected}
              className={cn(
                WORK_SELECTION_BUTTON_CLASS,
                isSelected && "border-af-accent/35 bg-af-accent/10 text-af-accent",
              )}
              key={workItem.work_id}
              onClick={() => onSelectWorkID?.(workItem.work_id)}
              type="button"
            >
              {workLabel}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function ScriptRequestFields({
  request,
}: {
  request: WorkstationRequestDetailCardProps["request"];
}) {
  const messages = useCurrentSelectionDetailMessages();
  const scriptRequest = request.script_request;
  if (!scriptRequest) {
    return null;
  }

  return (
    <>
      <div>
        <dt>{messages.scriptRequestIdLabel}</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {scriptRequest.script_request_id ? (
            <code className={RUNTIME_DETAIL_CODE_CLASS}>
              {scriptRequest.script_request_id}
            </code>
          ) : (
            messages.scriptRequestUnavailable
          )}
        </dd>
      </div>
      <div>
        <dt>{messages.scriptAttemptLabel}</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {scriptRequest.attempt ?? messages.scriptAttemptUnavailable}
        </dd>
      </div>
      <div>
        <dt>{messages.commandLabel}</dt>
        <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
          {scriptRequest.command ? (
            <code className={RUNTIME_DETAIL_CODE_CLASS}>
              {scriptRequest.command}
            </code>
          ) : (
            messages.commandUnavailable
          )}
        </dd>
      </div>
      <div>
        <dt>{messages.resolvedArgsLabel}</dt>
        <dd className="grid gap-1">
          {scriptRequest.args && scriptRequest.args.length > 0 ? (
            scriptRequest.args.map((arg: string) => (
              <code className={RUNTIME_DETAIL_CODE_CLASS} key={arg}>
                {arg}
              </code>
            ))
          ) : (
            <span className={RUNTIME_DETAIL_VALUE_CLASS}>
              {messages.scriptArgumentsUnavailable}
            </span>
          )}
        </dd>
      </div>
    </>
  );
}
