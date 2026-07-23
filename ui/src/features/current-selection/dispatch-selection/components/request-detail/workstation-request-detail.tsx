import { formatDurationMillis } from "../../../../../components/ui/formatters";
import { LocalizedTimezoneNote } from "../../../../../components/ui/localized-timezone-note";
import { SelectionDetailLayout } from "../../../base/components/layout/current-selection-detail-layout";
import {
  CurrentSelectionBodyLayout,
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailCode,
  CurrentSelectionDetailValue,
  CurrentSelectionExpandableSection,
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionShellMessages,
} from "../../../base/public";
import { getRunnerDisplayName } from "../../../editing/runner-metadata";
import {
  InferenceAttemptDetail,
  InferenceAttemptsSection,
  WorkItemPayloadList,
} from "../../../work-selection/public";
import type { WorkstationRequestDetailCardProps } from "../../lib/detail-card-types";
import { AgentRunInspectionSection } from "../sections/agent-run-inspection-section";
import { RequestMetadataSection } from "../sections/request-metadata-section";
import {
  ErrorDetailsSection,
  ResponseDetailsSection,
} from "../sections/workstation-request-detail-response";
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
  const messages = useCurrentSelectionDetailMessages();
  const shellMessages = useCurrentSelectionShellMessages();
  const view = buildWorkstationRequestDetailView(request);
  const showInferenceAttempts =
    !view.isScriptBackedRequest && !view.isAgentBackedRequest;

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={view.requestTitle}>
        <WorkstationRequestSummary request={request} view={view} />
        <RequestDetailsSection
          onSelectWorkID={onSelectWorkID}
          request={request}
          selectedWorkID={selectedWorkID}
          view={view}
        />
        {view.isScriptBackedRequest ? (
          <RequestMetadataSection
            emptyMessage={messages.metadataEmpty}
            metadata={request.request_metadata}
            title={messages.requestMetadataTitle}
          />
        ) : null}
        <ResponseDetailsSection request={request} view={view} />
        {view.isScriptBackedRequest ? (
          <RequestMetadataSection
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
        {view.isAgentBackedRequest ? (
          <AgentRunInspectionSection
            inspection={
              request.agent_run_inspection ??
              request.response_view?.agent_run_inspection ??
              request.response_view?.agentRunInspection
            }
          />
        ) : null}
        {showInferenceAttempts ? (
          <CurrentSelectionExpandableSection
            defaultExpanded
            title={shellMessages.inferenceAttemptsHeading}
            toggleLabel={(expanded) =>
              expanded ? messages.collapseAction : messages.expandAction
            }
          >
            <InferenceAttemptsSection
              attempts={request.inference_attempts}
              showHeading={false}
            />
          </CurrentSelectionExpandableSection>
        ) : null}
      </CurrentSelectionBodyLayout>
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
  const locale = useCurrentSelectionLocale();

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      title={messages.summaryTitle}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <LocalizedTimezoneNote
        locale={locale}
        timezoneLabel={messages.localizedTimezoneLabel}
      >
        {messages.localizedTimezoneContext}
      </LocalizedTimezoneNote>
      <CurrentSelectionDescriptionList>
        <InferenceAttemptDetail
          code
          label={messages.dispatchIdLabel}
          value={request.dispatch_id}
        />
        {request.request_id ? (
          <InferenceAttemptDetail
            code
            label={messages.requestIdLabel}
            value={request.request_id}
          />
        ) : null}
        <InferenceAttemptDetail
          label={messages.workstationLabel}
          value={request.workstation_name || messages.workstationUnavailable}
        />
        <div>
          <dt>{messages.outcomeLabel}</dt>
          <CurrentSelectionDetailValue>
            {view.outcome ? (
              <span className="flex flex-wrap gap-x-2 gap-y-1">
                <span>{view.outcome}</span>
                {view.hasFailedOutcome && view.normalizedFailureReason ? (
                  <span>
                    {messages.failureReasonLabel}:{" "}
                    {view.normalizedFailureReason}
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
          </CurrentSelectionDetailValue>
        </div>
        <InferenceAttemptDetail
          label={messages.totalDurationLabel}
          value={
            view.totalDurationMillis !== undefined
              ? formatDurationMillis(view.totalDurationMillis, locale)
              : messages.totalDurationUnavailable
          }
        />
        {view.requestRunner?.runnerId ? (
          <InferenceAttemptDetail
            label={messages.runnerLabel}
            value={
              getRunnerDisplayName(view.requestRunner.runnerId) ??
              view.requestRunner.displayName ??
              view.requestRunner.runnerId
            }
          />
        ) : null}
        {view.requestRunner?.selectionSource ? (
          <InferenceAttemptDetail
            label={messages.runnerSelectionSourceLabel}
            value={view.requestRunner.selectionSource}
          />
        ) : null}
      </CurrentSelectionDescriptionList>
    </CurrentSelectionExpandableSection>
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
      <CurrentSelectionExpandableSection
        defaultExpanded
        title={messages.requestDetailsTitle}
        toggleLabel={(expanded) =>
          expanded ? messages.collapseAction : messages.expandAction
        }
      >
        <ConsumedWorkItemsSection
          onSelectWorkID={onSelectWorkID}
          selectedWorkID={selectedWorkID}
          workItems={consumedWorkItems}
        />
      </CurrentSelectionExpandableSection>
    );
  }

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      title={messages.requestDetailsTitle}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <CurrentSelectionDescriptionList>
        <ScriptRequestFields request={request} />
      </CurrentSelectionDescriptionList>
      <ConsumedWorkItemsSection
        onSelectWorkID={onSelectWorkID}
        selectedWorkID={selectedWorkID}
        workItems={consumedWorkItems}
      />
    </CurrentSelectionExpandableSection>
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
    <WorkItemPayloadList
      messages={messages}
      onSelectWorkID={onSelectWorkID}
      selectedWorkID={selectedWorkID}
      workItems={workItems}
    />
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
        <CurrentSelectionDetailValue>
          {scriptRequest.script_request_id ? (
            <CurrentSelectionDetailCode>
              {scriptRequest.script_request_id}
            </CurrentSelectionDetailCode>
          ) : (
            messages.scriptRequestUnavailable
          )}
        </CurrentSelectionDetailValue>
      </div>
      <div>
        <dt>{messages.scriptAttemptLabel}</dt>
        <CurrentSelectionDetailValue>
          {scriptRequest.attempt ?? messages.scriptAttemptUnavailable}
        </CurrentSelectionDetailValue>
      </div>
      <div>
        <dt>{messages.commandLabel}</dt>
        <CurrentSelectionDetailValue>
          {scriptRequest.command ? (
            <CurrentSelectionDetailCode>
              {scriptRequest.command}
            </CurrentSelectionDetailCode>
          ) : (
            messages.commandUnavailable
          )}
        </CurrentSelectionDetailValue>
      </div>
      <div>
        <dt>{messages.resolvedArgsLabel}</dt>
        <dd className="grid gap-1">
          {scriptRequest.args && scriptRequest.args.length > 0 ? (
            scriptRequest.args.map((arg: string) => (
              <CurrentSelectionDetailCode key={arg}>
                {arg}
              </CurrentSelectionDetailCode>
            ))
          ) : (
            <span className="min-w-0 [overflow-wrap:anywhere]">
              {messages.scriptArgumentsUnavailable}
            </span>
          )}
        </dd>
      </div>
    </>
  );
}
