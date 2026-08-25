// biome-ignore-all lint/style/noExcessiveLinesPerFile: factory session detail panel composes runtime, drilldown, lifecycle, and replay sections.

import { useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Heading,
  Label,
  Text,
} from "@you-agent-factory/components/primitives";
import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { ReactNode } from "react";
import { useId, useState } from "react";
import { isDurableJavaScriptSession } from "../../../api/factory-sessions/normalize-durable-inspection";
import type { components } from "../../../api/generated/openapi";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { AlertPanel } from "../../../components/ui/alert-panel";
import { DashboardStatusPill } from "../../../components/ui/dashboard-status-pill";
import { DurabilityConfirmationState } from "../../../components/ui/durability-confirmation-state";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import type { FactorySessionArtifactDrilldownViewState } from "../hooks/use-factory-session-artifact-drilldown";
import {
  type FactorySessionDetailViewState,
  useFactorySessionDetail,
} from "../hooks/use-factory-session-detail";
import {
  FACTORY_SESSION_DISPATCH_DETAIL_QUERY_KEY,
  type FactorySessionDispatchDetailViewState,
  useFactorySessionDispatchDetail,
} from "../hooks/use-factory-session-dispatch-detail";
import type { FactorySessionEventReplayViewState } from "../hooks/use-factory-session-event-replay";
import type { FactorySessionLifecycleControl } from "../hooks/use-factory-session-lifecycle-control";
import { useFactorySessionLifecycleControl } from "../hooks/use-factory-session-lifecycle-control";
import { resolveFactorySessionLifecycleActionAvailability } from "../lib/factory-session-lifecycle-controls";
import { getFactorySessionDetailMessages } from "../messages/factory-session-detail";
import {
  formatFactoryOrchestratorKind,
  formatFactorySessionRuntimeStatus,
  formatFactorySessionScriptStatus,
  resolveFactoryDispatchStatusTone,
} from "../messages/factory-session-runtime-display";
import { FactorySessionArtifactList } from "./artifact-drilldown/factory-session-artifact-list";
import { DispatchDetailContent } from "./dispatch-detail/dispatch-detail-content";
import { FactorySessionEventReplayDisclosure } from "./event-replay/factory-session-event-replay-disclosure";
import { LifecycleActionSection } from "./lifecycle/lifecycle-action-section";

type FactoryArtifact = components["schemas"]["FactoryArtifact"];
type FactoryDispatch = components["schemas"]["FactoryDispatch"];

export interface FactorySessionDetailInspectionState {
  artifactDrilldowns?: Record<string, FactorySessionArtifactDrilldownViewState>;
  dispatchDetails?: Record<string, FactorySessionDispatchDetailViewState>;
  eventReplay?: FactorySessionEventReplayViewState;
  onRetryDispatchDetail?: (dispatchID: string) => void;
}

export interface FactorySessionDetailPanelProps {
  detailState?: FactorySessionDetailViewState;
  inspectionState?: FactorySessionDetailInspectionState;
  lifecycleControl?: FactorySessionLifecycleControl;
  locale?: string;
  sessionID: string | null;
}

export function FactorySessionDetailPanel({
  detailState,
  inspectionState,
  lifecycleControl,
  locale,
  sessionID,
}: FactorySessionDetailPanelProps) {
  if (sessionID === null || sessionID.trim() === "") {
    return null;
  }

  if (detailState !== undefined) {
    return (
      <FactorySessionDetailPanelContent
        detailState={detailState}
        inspectionState={inspectionState}
        lifecycleControl={lifecycleControl}
        locale={locale}
        sessionID={sessionID}
      />
    );
  }

  return (
    <FactorySessionDetailPanelWithQuery
      inspectionState={inspectionState}
      lifecycleControl={lifecycleControl}
      locale={locale}
      sessionID={sessionID}
    />
  );
}

function FactorySessionDetailPanelWithQuery({
  inspectionState,
  lifecycleControl,
  locale,
  sessionID,
}: {
  inspectionState?: FactorySessionDetailInspectionState;
  lifecycleControl?: FactorySessionLifecycleControl;
  locale?: string;
  sessionID: string;
}) {
  const detailState = useFactorySessionDetail(sessionID);

  return (
    <FactorySessionDetailPanelContent
      detailState={detailState}
      inspectionState={inspectionState}
      lifecycleControl={lifecycleControl}
      locale={locale}
      sessionID={sessionID}
    />
  );
}

function FactorySessionDetailPanelContent({
  detailState,
  inspectionState,
  lifecycleControl,
  locale,
  sessionID,
}: {
  detailState: FactorySessionDetailViewState;
  inspectionState?: FactorySessionDetailInspectionState;
  lifecycleControl?: FactorySessionLifecycleControl;
  locale?: string;
  sessionID: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <section
      aria-label={messages.selectedSessionHeading}
      className="grid gap-4"
    >
      <Heading>{messages.selectedSessionHeading}</Heading>
      <div className="grid gap-2">
        <Label>{messages.sessionIdLabel}</Label>
        <Text>{sessionID}</Text>
      </div>

      {detailState.status === "loading" ? (
        <StatusNotice>{messages.loadingState}</StatusNotice>
      ) : null}
      {detailState.status === "not-found" ? (
        <StatusNotice>{messages.missingState}</StatusNotice>
      ) : null}
      {detailState.status === "error" ? (
        <StatusNotice tone="error">
          {detailState.message ?? messages.errorState}
        </StatusNotice>
      ) : null}
      {detailState.status === "success" ? (
        <FactorySessionRuntimeSections
          data={detailState.data}
          inspectionState={inspectionState}
          lifecycleControl={lifecycleControl}
          locale={locale}
        />
      ) : null}
    </section>
  );
}

function FactorySessionRuntimeSections({
  data,
  inspectionState,
  lifecycleControl,
  locale,
}: {
  data: {
    dispatches?: FactoryDispatch[];
    durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"];
    durableReadModel?: components["schemas"]["FactorySessionDurableReadModel"];
    partialResult?: components["schemas"]["FactorySessionPartialResult"];
    result?: components["schemas"]["FactorySessionLiveResult"];
    session: components["schemas"]["FactorySession"];
  };
  inspectionState?: FactorySessionDetailInspectionState;
  lifecycleControl?: FactorySessionLifecycleControl;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const runtime = data.session.runtime;

  return (
    <div className="grid gap-4">
      <Heading>{messages.runtimeHeading}</Heading>
      <div className="grid gap-2 sm:grid-cols-2">
        <Metric
          label={messages.orchestratorKindLabel}
          value={formatFactoryOrchestratorKind(
            runtime.orchestratorKind,
            locale,
          )}
        />
        <Metric
          label={messages.statusLabel}
          value={formatFactorySessionRuntimeStatus(
            runtime.status,
            data.durableLifecycleStatus,
            locale,
          )}
        />
        {runtime.lifecycleControlStatus ? (
          <Metric
            label={messages.lifecycleControlStatusLabel}
            value={runtime.lifecycleControlStatus}
          />
        ) : null}
      </div>

      {data.durableReadModel ? (
        <DurableIntrospectionSummary
          durable={data.durableReadModel}
          locale={locale}
        />
      ) : null}

      {runtime.orchestratorKind === FactoryOrchestratorKind.JAVASCRIPT ? (
        <JavaScriptSessionProjection
          artifacts={runtime.artifacts}
          durableLifecycleStatus={data.durableLifecycleStatus}
          dispatches={data.dispatches}
          inspectionState={inspectionState}
          lifecycleControl={lifecycleControl}
          javascript={runtime.javascript}
          locale={locale}
          partialResult={data.partialResult}
          result={data.result}
          sessionID={data.session.id}
        />
      ) : (
        <PetriSessionProjection locale={locale} petri={runtime.petri} />
      )}
    </div>
  );
}

function DurableIntrospectionSummary({
  durable,
  locale,
}: {
  durable: components["schemas"]["FactorySessionDurableReadModel"];
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const resultAvailability = durable.resultSummary?.resultStatus
    ? messages.resultAvailabilityValue(durable.resultSummary.resultStatus)
    : messages.unavailableValue;

  return (
    <section aria-label={messages.introspectionHeading} className="grid gap-3">
      <Label>{messages.introspectionHeading}</Label>
      <div className="grid gap-2 sm:grid-cols-2">
        <Metric
          label={messages.sourceLabel}
          value={durable.resolvedSource.sourceRef ?? messages.unavailableValue}
        />
        <Metric
          label={messages.sourceHashLabel}
          value={
            durable.sourceHash ??
            durable.resolvedSource.sourceHash ??
            messages.unavailableValue
          }
        />
        <Metric
          label={messages.latestCheckpointLabel}
          value={formatCheckpoint(
            durable.latestCheckpoint,
            messages.unavailableValue,
          )}
        />
        <Metric
          label={messages.effectivePolicyLabel}
          value={formatRecordSummary(
            durable.effectivePolicy,
            messages.unavailableValue,
          )}
        />
        <Metric
          label={messages.budgetLabel}
          value={formatRecordSummary(
            durable.budgets,
            messages.unavailableValue,
          )}
        />
        <Metric
          label={messages.usageHeading}
          value={formatRecordSummary(durable.usage, messages.unavailableValue)}
        />
        <Metric
          label={messages.resultAvailabilityLabel}
          value={resultAvailability}
        />
        <Metric
          label={messages.dispatchCountsLabel}
          value={formatRecordSummary(
            durable.progress,
            messages.unavailableValue,
          )}
        />
      </div>
      <PhaseSummaryList durable={durable} locale={locale} />
    </section>
  );
}

function PhaseSummaryList({
  durable,
  locale,
}: {
  durable: components["schemas"]["FactorySessionDurableReadModel"];
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const phases = durable.phaseSummaries ?? [];

  return (
    <div className="grid gap-2">
      <Label>{messages.phaseSummariesHeading}</Label>
      {phases.length === 0 ? (
        <Text>{messages.noneValue}</Text>
      ) : (
        <ol className="grid gap-2">
          {phases.map((phase) => (
            <li
              className="rounded-lg border border-outline p-3"
              key={phase.phase}
            >
              <Text>
                {phase.label ? `${phase.label} (${phase.phase})` : phase.phase}
                {durable.phase === phase.phase
                  ? ` — ${messages.currentPhaseValue}`
                  : ""}
              </Text>
              <Text variant="supporting">
                {messages.phaseDispatchSummary({
                  completed: phase.completedDispatchCount ?? 0,
                  failed: phase.failedDispatchCount ?? 0,
                  total: phase.dispatchCount ?? 0,
                })}
              </Text>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}

function JavaScriptSessionProjection({
  artifacts,
  durableLifecycleStatus,
  dispatches,
  inspectionState,
  lifecycleControl,
  javascript,
  locale,
  partialResult,
  result,
  sessionID,
}: {
  artifacts?: FactoryArtifact[];
  durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"];
  dispatches?: FactoryDispatch[];
  inspectionState?: FactorySessionDetailInspectionState;
  lifecycleControl?: FactorySessionLifecycleControl;
  javascript?: components["schemas"]["FactorySessionJavaScriptProjection"];
  locale?: string;
  partialResult?: components["schemas"]["FactorySessionPartialResult"];
  result?: components["schemas"]["FactorySessionLiveResult"];
  sessionID: string;
}) {
  const [selectedDispatchID, setSelectedDispatchID] = useState<string | null>(
    null,
  );
  const isDurableSession = isDurableJavaScriptSession(
    sessionID,
    FactoryOrchestratorKind.JAVASCRIPT,
    durableLifecycleStatus,
  );
  const lifecycleActionAvailability =
    resolveFactorySessionLifecycleActionAvailability({
      durableLifecycleStatus,
      dispatches,
      isDurableSession,
      selectedDispatchID,
    });

  const projectionProps = {
    artifacts,
    dispatches,
    inspectionState,
    isDurableSession,
    javascript,
    lifecycleActionAvailability,
    locale,
    partialResult,
    result,
    selectedDispatchID,
    sessionID,
    setSelectedDispatchID,
  };

  if (lifecycleControl !== undefined) {
    return (
      <JavaScriptSessionProjectionContent
        {...projectionProps}
        lifecycleControl={lifecycleControl}
      />
    );
  }

  return (
    <JavaScriptSessionProjectionWithQueryControl
      {...projectionProps}
      selectedDispatchIDForControl={
        lifecycleActionAvailability.selectedDispatch?.id ?? null
      }
    />
  );
}

function JavaScriptSessionProjectionWithQueryControl({
  selectedDispatchIDForControl,
  sessionID,
  ...projectionProps
}: Omit<JavaScriptSessionProjectionContentProps, "lifecycleControl"> & {
  selectedDispatchIDForControl: string | null;
}) {
  const lifecycleControl = useFactorySessionLifecycleControl({
    selectedDispatchID: selectedDispatchIDForControl,
    sessionID,
  });

  return (
    <JavaScriptSessionProjectionContent
      {...projectionProps}
      lifecycleControl={lifecycleControl}
      sessionID={sessionID}
    />
  );
}

interface JavaScriptSessionProjectionContentProps {
  artifacts?: FactoryArtifact[];
  dispatches?: FactoryDispatch[];
  inspectionState?: FactorySessionDetailInspectionState;
  isDurableSession: boolean;
  javascript?: components["schemas"]["FactorySessionJavaScriptProjection"];
  lifecycleActionAvailability: ReturnType<
    typeof resolveFactorySessionLifecycleActionAvailability
  >;
  locale?: string;
  partialResult?: components["schemas"]["FactorySessionPartialResult"];
  result?: components["schemas"]["FactorySessionLiveResult"];
  selectedDispatchID: string | null;
  sessionID: string;
  setSelectedDispatchID: (dispatchID: string | null) => void;
}

function JavaScriptSessionProjectionContent({
  artifacts,
  dispatches,
  inspectionState,
  isDurableSession,
  javascript,
  lifecycleActionAvailability,
  lifecycleControl,
  locale,
  partialResult,
  result,
  selectedDispatchID,
  sessionID,
  setSelectedDispatchID,
}: JavaScriptSessionProjectionContentProps & {
  lifecycleControl: FactorySessionLifecycleControl;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  if (!javascript) {
    return (
      <WidgetDetailCopy>
        {messages.javascriptProjectionMissingState}
      </WidgetDetailCopy>
    );
  }

  const warnings = (dispatches ?? []).flatMap(
    (dispatch) => dispatch.warnings ?? [],
  );

  return (
    <div className="grid gap-4">
      <div className="grid gap-2 sm:grid-cols-2">
        {javascript.phase ? (
          <Metric label={messages.phaseLabel} value={javascript.phase} />
        ) : null}
        <Metric
          label={messages.scriptStatusLabel}
          value={formatFactorySessionScriptStatus(
            javascript.scriptStatus,
            locale,
          )}
        />
        <Metric
          label={messages.childDispatchCountsLabel}
          value={`queued ${javascript.childDispatchCounts.queued}, running ${javascript.childDispatchCounts.running}, completed ${javascript.childDispatchCounts.completed}`}
        />
      </div>
      {isDurableSession ? (
        <LifecycleActionSection
          availability={lifecycleActionAvailability}
          feedback={lifecycleControl.feedback}
          locale={locale}
          onAction={lifecycleControl.submitLifecycleAction}
          pendingActionID={lifecycleControl.pendingActionID}
        />
      ) : null}
      {javascript.phases.length > 0 ? (
        <Metric
          label={messages.phasesLabel}
          value={javascript.phases.join(", ")}
        />
      ) : null}

      {javascript.checkpoints && javascript.checkpoints.length > 0 ? (
        <CheckpointRefList
          checkpoints={javascript.checkpoints}
          heading={messages.checkpointRefsHeading}
        />
      ) : null}

      {isDurableSession ? (
        <FactorySessionEventReplayDisclosure
          locale={locale}
          replayState={inspectionState?.eventReplay}
          sessionID={sessionID}
        />
      ) : null}

      {artifacts && artifacts.length > 0 ? (
        <FactorySessionArtifactList
          artifacts={artifacts}
          drilldownStates={inspectionState?.artifactDrilldowns}
          heading={messages.artifactsHeading}
          locale={locale}
          sessionID={sessionID}
        />
      ) : null}

      {warnings.length > 0 ? (
        <WarningList heading={messages.warningsHeading} warnings={warnings} />
      ) : null}

      {dispatches && dispatches.length > 0 ? (
        <DispatchSummaryList
          dispatches={dispatches}
          inspectionState={inspectionState}
          locale={locale}
          selectedDispatchID={selectedDispatchID}
          setSelectedDispatchID={setSelectedDispatchID}
          sessionID={result?.sessionId ?? partialResult?.sessionId}
        />
      ) : null}

      {result?.resultArtifactRef ? (
        <Metric
          label={messages.finalResultRefLabel}
          value={formatArtifactRef(result.resultArtifactRef)}
        />
      ) : null}
      {partialResult?.partialResultArtifactRef ? (
        <Metric
          label={messages.partialResultRefLabel}
          value={formatArtifactRef(partialResult.partialResultArtifactRef)}
        />
      ) : null}
    </div>
  );
}

function DispatchSummaryList({
  dispatches,
  inspectionState,
  locale,
  selectedDispatchID,
  sessionID,
  setSelectedDispatchID,
}: {
  dispatches: FactoryDispatch[];
  inspectionState?: FactorySessionDetailInspectionState;
  locale?: string;
  selectedDispatchID: string | null;
  sessionID?: string;
  setSelectedDispatchID: (dispatchID: string | null) => void;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <section className="grid gap-3">
      <Label>{messages.dispatchesHeading}</Label>
      <WidgetDetailCopy>{messages.dispatchSelectionHint}</WidgetDetailCopy>
      <div className="grid gap-3">
        {dispatches.map((dispatch) => (
          <DispatchSummaryRow
            dispatch={dispatch}
            expanded={selectedDispatchID === dispatch.id}
            detailState={
              inspectionState?.dispatchDetails
                ? (inspectionState.dispatchDetails[dispatch.id] ?? {
                    status: "idle",
                  })
                : undefined
            }
            key={dispatch.id}
            locale={locale}
            onToggle={(expanded) =>
              setSelectedDispatchID(expanded ? dispatch.id : null)
            }
            onRetry={
              inspectionState?.onRetryDispatchDetail
                ? () => inspectionState.onRetryDispatchDetail?.(dispatch.id)
                : undefined
            }
            sessionID={sessionID ?? dispatch.sessionId}
          />
        ))}
      </div>
    </section>
  );
}

function DispatchSummaryRow({
  dispatch,
  detailState,
  expanded,
  locale,
  onRetry,
  onToggle,
  sessionID,
}: {
  dispatch: FactoryDispatch;
  detailState?: FactorySessionDispatchDetailViewState;
  expanded: boolean;
  locale?: string;
  onRetry?: () => void;
  onToggle: (expanded: boolean) => void;
  sessionID: string;
}) {
  if (detailState !== undefined) {
    return (
      <DispatchSummaryRowContent
        detailState={detailState}
        dispatch={dispatch}
        expanded={expanded}
        locale={locale}
        onRetry={onRetry ?? (() => undefined)}
        onToggle={onToggle}
      />
    );
  }

  return (
    <DispatchSummaryRowWithQuery
      dispatch={dispatch}
      expanded={expanded}
      locale={locale}
      onToggle={onToggle}
      sessionID={sessionID}
    />
  );
}

function DispatchSummaryRowWithQuery({
  dispatch,
  expanded,
  locale,
  onToggle,
  sessionID,
}: Omit<DispatchSummaryRowProps, "detailState" | "onRetry">) {
  const queryClient = useQueryClient();
  const detailState = useFactorySessionDispatchDetail(
    sessionID,
    expanded ? dispatch.id : null,
  );
  const handleRetryDispatchDetail = () => {
    void queryClient.refetchQueries({
      queryKey: [
        ...FACTORY_SESSION_DISPATCH_DETAIL_QUERY_KEY,
        sessionID,
        dispatch.id,
      ],
    });
  };

  return (
    <DispatchSummaryRowContent
      detailState={detailState}
      dispatch={dispatch}
      expanded={expanded}
      locale={locale}
      onRetry={handleRetryDispatchDetail}
      onToggle={onToggle}
    />
  );
}

interface DispatchSummaryRowProps {
  dispatch: FactoryDispatch;
  detailState?: FactorySessionDispatchDetailViewState;
  expanded: boolean;
  locale?: string;
  onRetry?: () => void;
  onToggle: (expanded: boolean) => void;
  sessionID: string;
}

function DispatchSummaryRowContent({
  detailState,
  dispatch,
  expanded,
  locale,
  onRetry,
  onToggle,
}: Omit<DispatchSummaryRowProps, "detailState" | "sessionID"> & {
  detailState: FactorySessionDispatchDetailViewState;
  onRetry: () => void;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const detailRegionID = useId();
  const dispatchLabel = dispatch.label?.trim() || dispatch.id;
  const summaryDetails = getDispatchSummaryDetails(dispatch, messages);

  return (
    <article className="grid gap-3 rounded-lg border border-outline bg-surface-container-low p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="grid min-w-0 gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Label>{dispatchLabel}</Label>
            <DashboardStatusPill
              size="compact"
              tone={resolveFactoryDispatchStatusTone({
                status: dispatch.status,
                warningCount: dispatch.warnings?.length ?? 0,
              })}
            >
              {dispatch.status}
            </DashboardStatusPill>
            <DurabilityConfirmationState
              label={messages.durabilityConfirmationLabel}
              state={dispatch.confirmationState}
            />
          </div>
          <Text
            as="div"
            className="flex flex-wrap items-center gap-x-3 gap-y-1 text-on-surface-subtle"
            variant="supporting"
          >
            <span>{dispatch.dispatchKind}</span>
            <span>{dispatch.id}</span>
          </Text>
          {summaryDetails.length > 0 ? (
            <Text
              as="div"
              className="flex flex-wrap items-center gap-x-3 gap-y-1 text-on-surface-subtle"
              variant="supporting"
            >
              {summaryDetails.map((detail) => (
                <span key={detail}>{detail}</span>
              ))}
            </Text>
          ) : null}
        </div>
        <ExpandablePanelTrigger
          aria-label={
            expanded
              ? messages.collapseDispatchDetailLabel(dispatch.id)
              : messages.expandDispatchDetailLabel(dispatch.id)
          }
          controlsID={detailRegionID}
          expanded={expanded}
          onClick={() => onToggle(!expanded)}
          variant="compact"
        >
          {messages.dispatchDetailHeading}
        </ExpandablePanelTrigger>
      </div>

      {expanded ? (
        <DispatchDetailState
          detailRegionID={detailRegionID}
          dispatchID={dispatch.id}
          locale={locale}
          onRetry={onRetry}
          state={detailState}
        />
      ) : null}
    </article>
  );
}

function DispatchDetailState({
  detailRegionID,
  dispatchID,
  locale,
  onRetry,
  state,
}: {
  detailRegionID: string;
  dispatchID: string;
  locale?: string;
  onRetry: () => void;
  state: ReturnType<typeof useFactorySessionDispatchDetail>;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  if (state.status === "loading") {
    return (
      <WidgetDetailCopy id={detailRegionID} role="status">
        {messages.dispatchDetailLoadingState}
      </WidgetDetailCopy>
    );
  }

  if (state.status === "not-found") {
    return (
      <WidgetDetailCopy id={detailRegionID} role="status">
        {messages.dispatchDetailMissingState(dispatchID)}
      </WidgetDetailCopy>
    );
  }

  if (state.status === "error") {
    return (
      <AlertPanel id={detailRegionID} tone="danger">
        <div className="grid gap-3">
          <p>{state.message ?? messages.dispatchDetailErrorState}</p>
          <Button onClick={onRetry} size="sm" tone="outline">
            {messages.dispatchDetailRetryLabel}
          </Button>
        </div>
      </AlertPanel>
    );
  }

  if (state.status !== "success") {
    return null;
  }

  return (
    <div
      className="grid gap-3 border-t border-outline pt-3"
      id={detailRegionID}
    >
      <DispatchDetailContent data={state.data} locale={locale} />
    </div>
  );
}

function PetriSessionProjection({
  locale,
  petri,
}: {
  locale?: string;
  petri?: components["schemas"]["FactorySessionPetriProjection"];
}) {
  const messages = getFactorySessionDetailMessages(locale);

  if (!petri) {
    return <WidgetDetailCopy>{messages.markingEmptyState}</WidgetDetailCopy>;
  }

  return (
    <div className="grid gap-4">
      <Metric
        label={messages.markingHeading}
        value={`${petri.marking.length} token${petri.marking.length === 1 ? "" : "s"}`}
      />
      {petri.enabledTransitions.length > 0 ? (
        <div className="grid gap-2">
          <Label>{messages.enabledTransitionsHeading}</Label>
          <ul className="grid gap-1">
            {petri.enabledTransitions.map((transition) => (
              <li key={transition.transitionId}>
                <Text>
                  {transition.transitionId} ({transition.workerType})
                </Text>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

function CheckpointRefList({
  checkpoints,
  heading,
}: {
  checkpoints: components["schemas"]["FactorySessionJavaScriptCheckpointRef"][];
  heading: string;
}) {
  return (
    <div className="grid gap-2">
      <Label>{heading}</Label>
      <ul className="grid gap-1">
        {checkpoints.map((checkpoint) => (
          <li key={checkpoint.id}>
            <Text>
              {checkpoint.label
                ? `${checkpoint.id} (${checkpoint.label})`
                : checkpoint.id}
              {checkpoint.summary ? ` — ${checkpoint.summary}` : ""}
            </Text>
          </li>
        ))}
      </ul>
    </div>
  );
}

function WarningList({
  heading,
  warnings,
}: {
  heading: string;
  warnings: components["schemas"]["FactoryDispatchWarning"][];
}) {
  return (
    <div className="grid gap-2">
      <Label>{heading}</Label>
      <ul className="grid gap-1">
        {warnings.map((warning) => (
          <li key={`${warning.code}:${warning.message}`}>
            <Text>{warning.message}</Text>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1">
      <Label>{label}</Label>
      <Text>{value}</Text>
    </div>
  );
}

function StatusNotice({
  children,
  tone = "default",
}: {
  children: ReactNode;
  tone?: "default" | "error";
}) {
  return (
    <AlertPanel
      radius="lg"
      role={tone === "error" ? "alert" : "status"}
      tone={tone === "error" ? "danger" : "info"}
    >
      {children}
    </AlertPanel>
  );
}

function formatArtifactRef(
  artifactRef: components["schemas"]["FactoryArtifactRef"],
): string {
  return `${artifactRef.id} · ${artifactRef.kind}`;
}

function formatCheckpoint(
  checkpoint: components["schemas"]["FactorySessionCheckpointRef"] | undefined,
  unavailable: string,
): string {
  if (!checkpoint) return unavailable;
  const label = checkpoint.label ? ` (${checkpoint.label})` : "";
  const phase = checkpoint.phase ? ` · ${checkpoint.phase}` : "";
  return `${checkpoint.id}${label}${phase}`;
}

function formatRecordSummary(value: unknown, unavailable: string): string {
  if (!value || typeof value !== "object") return unavailable;
  const entries = Object.entries(value).filter(
    ([, item]) => item !== undefined,
  );
  if (entries.length === 0) return unavailable;
  return entries
    .map(([key, item]) =>
      Array.isArray(item)
        ? `${key}: ${item.length === 0 ? "none" : item.length}`
        : `${key}: ${String(item)}`,
    )
    .join(" · ");
}

function getDispatchSummaryDetails(
  dispatch: FactoryDispatch,
  messages: ReturnType<typeof getFactorySessionDetailMessages>,
): string[] {
  const details: string[] = [];
  const executionMode = dispatch.javascript?.executionMode?.trim();
  if (executionMode) {
    details.push(messages.dispatchExecutionModeSummary(executionMode));
  }

  const firstProviderSessionRef = dispatch.providerSessionRefs?.[0];
  if (firstProviderSessionRef) {
    details.push(
      messages.dispatchProviderSessionSummary({
        id: firstProviderSessionRef.id,
        kind: firstProviderSessionRef.kind,
        provider: firstProviderSessionRef.provider?.trim() || undefined,
      }),
    );
  }

  return details;
}
