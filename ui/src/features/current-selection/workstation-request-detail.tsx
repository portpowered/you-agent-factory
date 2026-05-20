import { formatWorkItemLabel } from "../../components/ui/formatters";
import { formatDurationMillis } from "../../components/ui/formatters";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  MetadataSection,
  RequestCountSection,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
  WORK_SELECTION_BUTTON_CLASS,
} from "./detail-card-shared";
import type { WorkstationRequestDetailCardProps } from "./detail-card-types";
import { InferenceAttemptsSection } from "./execution-details";
import { useCurrentSelectionShellMessages } from "./current-selection-locale";
import {
  ErrorDetailsSection,
  ResponseDetailsSection,
} from "./workstation-request-detail-response";
import {
  buildWorkstationRequestDetailView,
  type WorkstationRequestDetailView,
} from "./workstation-request-detail-view";

export function WorkstationRequestDetailCard({
  onSelectWorkID,
  request,
  selectedWorkID,
  widgetId = "current-selection",
}: WorkstationRequestDetailCardProps) {
  const view = buildWorkstationRequestDetailView(request);
  const showInferenceAttempts = !view.isScriptBackedRequest;

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <WorkstationRequestSummary request={request} view={view} />
      <RequestCountSection request={request} />
      <RequestDetailsSection
        onSelectWorkID={onSelectWorkID}
        request={request}
        selectedWorkID={selectedWorkID}
        view={view}
      />
      {view.isScriptBackedRequest ? (
        <MetadataSection
          emptyMessage="Request metadata is not available for this workstation request."
          metadata={request.request_metadata}
          title="Request metadata"
        />
      ) : null}
      <ResponseDetailsSection request={request} view={view} />
      {view.isScriptBackedRequest ? (
        <MetadataSection
          emptyMessage={view.responseMetadataUnavailableCopy}
          metadata={request.response_metadata}
          title="Response metadata"
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
  const messages = useCurrentSelectionShellMessages();

  return (
    <>
      <p className={WIDGET_SUBTITLE_CLASS}>
        {request.request_id || request.dispatch_id}
      </p>
      <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
        <div>
          <dt>Dispatch ID</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            <code className={RUNTIME_DETAIL_CODE_CLASS}>
              {request.dispatch_id}
            </code>
          </dd>
        </div>
        {request.request_id ? (
          <div>
            <dt>Request ID</dt>
            <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
              <code className={RUNTIME_DETAIL_CODE_CLASS}>
                {request.request_id}
              </code>
            </dd>
          </div>
        ) : null}
        <div>
          <dt>Workstation</dt>
          <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
            {request.workstation_name ||
              "Workstation details are not available for this request."}
          </dd>
        </div>
        <div>
          <dt>Outcome</dt>
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
              "Outcome details are not available yet."
            )}
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
  const consumedWorkItems = request.request_view?.input_work_items ?? [];

  if (!view.isScriptBackedRequest) {
    return (
      <section
        aria-label="Request details"
        className={RUNTIME_DETAILS_SECTION_CLASS}
      >
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Request details</h4>
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
      aria-label="Request details"
      className={RUNTIME_DETAILS_SECTION_CLASS}
    >
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>Request details</h4>
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
  if (workItems.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-1">
      <span>Consumed work items</span>
      <div className="flex flex-wrap gap-2">
        {workItems.map((workItem) => {
          const workLabel = formatWorkItemLabel(workItem);
          const isSelected = selectedWorkID === workItem.work_id;

          return (
            <button
              aria-label={`Select work item ${workLabel}`}
              aria-pressed={isSelected}
              className={WORK_SELECTION_BUTTON_CLASS}
              key={workItem.work_id}
              onClick={() => onSelectWorkID?.(workItem.work_id)}
              type="button"
            >
              {isSelected ? workLabel : `Open ${workLabel}`}
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
        <dd className="grid gap-1">
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
