import { formatDurationMillis } from "../../../components/ui/formatters";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
} from "../../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  MetadataSection,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
} from "./detail-card-shared";
import type { WorkstationRequestDetailCardProps } from "../detail-card-types";
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
import { WorkItemPayloadList } from "./work-item-payload-details";
import {
  getRunnerDisplayName,
  resolveSelectedRunnerMetadata,
  type RunnerOptionalCapability,
} from "../runner-metadata";

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
  const requestRunnerMetadata = resolveSelectedRunnerMetadata(
    view.requestRunner,
  );

  return (
    <>
      <p className={WIDGET_SUBTITLE_CLASS}>{view.requestTitle}</p>
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
        {view.requestRunner?.runnerId ? (
          <div>
            <dt>{messages.runnerLabel}</dt>
            <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
              {getRunnerDisplayName(view.requestRunner.runnerId) ??
                view.requestRunner.displayName ??
                view.requestRunner.runnerId}
            </dd>
          </div>
        ) : null}
        {view.requestRunner?.selectionSource ? (
          <div>
            <dt>{messages.runnerSelectionSourceLabel}</dt>
            <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
              {view.requestRunner.selectionSource}
            </dd>
          </div>
        ) : null}
      </dl>
      {requestRunnerMetadata ? (
        <div className="mt-3 grid gap-2 rounded-xl border border-af-overlay/8 bg-af-overlay/4 p-3">
          <p className="m-0 text-sm text-af-ink/72">
            {messages.runnerCapabilitySupportHeading}
          </p>
          <ul className="m-0 grid list-none gap-2 p-0">
            {requestRunnerMetadata.capabilities.optionalCapabilities.map(
              (capability) => (
                <li
                  className="grid gap-1 rounded-lg border border-af-overlay/8 bg-af-surface/66 p-2"
                  key={capability.capability}
                >
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="text-sm text-af-ink">
                      {labelForRunnerCapability(
                        messages,
                        capability.capability,
                      )}
                    </span>
                    <span
                      className={
                        capability.status === "supported"
                          ? "rounded-full bg-af-success/10 px-2 py-1 text-xs font-semibold text-af-success-ink"
                          : "rounded-full bg-af-warning/12 px-2 py-1 text-xs font-semibold text-af-warning-ink"
                      }
                    >
                      {capability.status === "supported"
                        ? messages.runnerCapabilitySupportedLabel
                        : messages.runnerCapabilityUnsupportedLabel}
                    </span>
                  </div>
                  {capability.detail ? (
                    <p className="m-0 text-sm text-af-ink/62">
                      {capability.detail}
                    </p>
                  ) : null}
                </li>
              ),
            )}
          </ul>
        </div>
      ) : null}
    </>
  );
}

function labelForRunnerCapability(
  messages: ReturnType<typeof useCurrentSelectionDetailMessages>,
  capability: RunnerOptionalCapability,
) {
  switch (capability) {
    case "image_input":
      return messages.runnerCapabilityImageInputLabel;
    case "session_resume":
      return messages.runnerCapabilitySessionResumeLabel;
    case "structured_output":
      return messages.runnerCapabilityStructuredOutputLabel;
    case "working_directory":
      return messages.runnerCapabilityWorkingDirectoryLabel;
    case "worktree":
      return messages.runnerCapabilityWorktreeLabel;
  }
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
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.requestDetailsTitle}
        </h4>
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
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
        {messages.requestDetailsTitle}
      </h4>
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
