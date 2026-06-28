import { type ReactNode, useId, useState } from "react";

import { isDurableJavaScriptSession } from "../../../api/factory-sessions/normalize-durable-inspection";
import type { components } from "../../../api/generated/openapi";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import {
  AlertPanel,
  DashboardHeading,
  DashboardLabel,
  DashboardStatusPill,
  DashboardText,
} from "../../../components/ui";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import { DetailCopy } from "../../../components/ui/widget-frame";
import { FactorySessionArtifactList } from "./artifact-drilldown/factory-session-artifact-list";
import { useFactorySessionDetail } from "../hooks/use-factory-session-detail";
import { useFactorySessionLifecycleControl } from "../hooks/use-factory-session-lifecycle-control";
import { useFactorySessionDispatchDetail } from "../hooks/use-factory-session-dispatch-detail";
import { resolveFactorySessionLifecycleActionAvailability } from "../lib/factory-session-lifecycle-controls";
import { getFactorySessionDetailMessages } from "../messages/factory-session-detail";
import {
  formatFactoryOrchestratorKind,
  formatFactorySessionRuntimeStatus,
  formatFactorySessionScriptStatus,
} from "../messages/factory-session-runtime-display";
import { DispatchDetailContent } from "./dispatch-detail/dispatch-detail-content";
import { FactorySessionEventReplayDisclosure } from "./event-replay/factory-session-event-replay-disclosure";
import { LifecycleActionSection } from "./lifecycle/lifecycle-action-section";

type FactoryArtifact = components["schemas"]["FactoryArtifact"];
type FactoryDispatch = components["schemas"]["FactoryDispatch"];

export interface FactorySessionDetailPanelProps {
  locale?: string;
  sessionID: string | null;
}

export function FactorySessionDetailPanel({
  locale,
  sessionID,
}: FactorySessionDetailPanelProps) {
  const messages = getFactorySessionDetailMessages(locale);
  const detailState = useFactorySessionDetail(sessionID);

  if (sessionID === null || sessionID.trim() === "") {
    return null;
  }

  return (
    <section
      aria-label={messages.selectedSessionHeading}
      className="grid gap-4"
    >
      <DashboardHeading>{messages.selectedSessionHeading}</DashboardHeading>
      <div className="grid gap-2">
        <DashboardLabel>{messages.sessionIdLabel}</DashboardLabel>
        <DashboardText>{sessionID}</DashboardText>
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
          locale={locale}
        />
      ) : null}
    </section>
  );
}

function FactorySessionRuntimeSections({
  data,
  locale,
}: {
  data: {
    durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"];
    partialResult?: components["schemas"]["FactorySessionPartialResult"];
    result?: components["schemas"]["FactorySessionLiveResult"];
    session: components["schemas"]["FactorySession"];
  };
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const runtime = data.session.runtime;

  return (
    <div className="grid gap-4">
      <DashboardHeading>{messages.runtimeHeading}</DashboardHeading>
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
      </div>

      {runtime.orchestratorKind === FactoryOrchestratorKind.JAVASCRIPT ? (
        <JavaScriptSessionProjection
          artifacts={runtime.artifacts}
          durableLifecycleStatus={data.durableLifecycleStatus}
          dispatches={runtime.dispatches}
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

function JavaScriptSessionProjection({
  artifacts,
  durableLifecycleStatus,
  dispatches,
  javascript,
  locale,
  partialResult,
  result,
  sessionID,
}: {
  artifacts?: FactoryArtifact[];
  durableLifecycleStatus?: components["schemas"]["FactorySessionDurableLifecycleStatus"];
  dispatches?: FactoryDispatch[];
  javascript?: components["schemas"]["FactorySessionJavaScriptProjection"];
  locale?: string;
  partialResult?: components["schemas"]["FactorySessionPartialResult"];
  result?: components["schemas"]["FactorySessionLiveResult"];
  sessionID: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
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
  const lifecycleControl = useFactorySessionLifecycleControl({
    selectedDispatchID: lifecycleActionAvailability.selectedDispatch?.id ?? null,
    sessionID,
  });

  if (!javascript) {
    return <DetailCopy>{messages.javascriptProjectionMissingState}</DetailCopy>;
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
          sessionID={sessionID}
        />
      ) : null}

      {artifacts && artifacts.length > 0 ? (
        <FactorySessionArtifactList
          artifacts={artifacts}
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
  locale,
  selectedDispatchID,
  sessionID,
  setSelectedDispatchID,
}: {
  dispatches: FactoryDispatch[];
  locale?: string;
  selectedDispatchID: string | null;
  sessionID?: string;
  setSelectedDispatchID: (dispatchID: string | null) => void;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <section className="grid gap-3">
      <DashboardLabel>{messages.dispatchesHeading}</DashboardLabel>
      <DetailCopy>{messages.dispatchSelectionHint}</DetailCopy>
      <div className="grid gap-3">
        {dispatches.map((dispatch) => (
          <DispatchSummaryRow
            dispatch={dispatch}
            expanded={selectedDispatchID === dispatch.id}
            key={dispatch.id}
            locale={locale}
            onToggle={(expanded) =>
              setSelectedDispatchID(expanded ? dispatch.id : null)
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
  expanded,
  locale,
  onToggle,
  sessionID,
}: {
  dispatch: FactoryDispatch;
  expanded: boolean;
  locale?: string;
  onToggle: (expanded: boolean) => void;
  sessionID: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const detailRegionID = useId();
  const detailState = useFactorySessionDispatchDetail(
    sessionID,
    expanded ? dispatch.id : null,
  );
  const dispatchLabel = dispatch.label?.trim() || dispatch.id;
  const summaryDetails = getDispatchSummaryDetails(dispatch, messages);

  return (
    <article className="grid gap-3 rounded-lg border border-outline bg-surface-container-low p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="grid min-w-0 gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <DashboardLabel>{dispatchLabel}</DashboardLabel>
            <DashboardStatusPill size="compact">
              {dispatch.status}
            </DashboardStatusPill>
          </div>
          <DashboardText
            as="div"
            className="flex flex-wrap items-center gap-x-3 gap-y-1 text-on-surface-subtle"
            variant="supporting"
          >
            <span>{dispatch.dispatchKind}</span>
            <span>{dispatch.id}</span>
          </DashboardText>
          {summaryDetails.length > 0 ? (
            <DashboardText
              as="div"
              className="flex flex-wrap items-center gap-x-3 gap-y-1 text-on-surface-subtle"
              variant="supporting"
            >
              {summaryDetails.map((detail) => (
                <span key={detail}>{detail}</span>
              ))}
            </DashboardText>
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
          locale={locale}
          state={detailState}
        />
      ) : null}
    </article>
  );
}

function DispatchDetailState({
  detailRegionID,
  locale,
  state,
}: {
  detailRegionID: string;
  locale?: string;
  state: ReturnType<typeof useFactorySessionDispatchDetail>;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  if (state.status === "loading") {
    return (
      <DetailCopy id={detailRegionID}>
        {messages.dispatchDetailLoadingState}
      </DetailCopy>
    );
  }

  if (state.status === "not-found") {
    return (
      <DetailCopy id={detailRegionID}>
        {messages.dispatchDetailMissingState}
      </DetailCopy>
    );
  }

  if (state.status === "error") {
    return (
      <AlertPanel id={detailRegionID} tone="danger">
        {state.message ?? messages.dispatchDetailErrorState}
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
    return <DetailCopy>{messages.markingEmptyState}</DetailCopy>;
  }

  return (
    <div className="grid gap-4">
      <Metric
        label={messages.markingHeading}
        value={`${petri.marking.length} token${petri.marking.length === 1 ? "" : "s"}`}
      />
      {petri.enabledTransitions.length > 0 ? (
        <div className="grid gap-2">
          <DashboardLabel>{messages.enabledTransitionsHeading}</DashboardLabel>
          <ul className="grid gap-1">
            {petri.enabledTransitions.map((transition) => (
              <li key={transition.transitionId}>
                <DashboardText>
                  {transition.transitionId} ({transition.workerType})
                </DashboardText>
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
      <DashboardLabel>{heading}</DashboardLabel>
      <ul className="grid gap-1">
        {checkpoints.map((checkpoint) => (
          <li key={checkpoint.id}>
            <DashboardText>
              {checkpoint.label
                ? `${checkpoint.id} (${checkpoint.label})`
                : checkpoint.id}
              {checkpoint.summary ? ` — ${checkpoint.summary}` : ""}
            </DashboardText>
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
      <DashboardLabel>{heading}</DashboardLabel>
      <ul className="grid gap-1">
        {warnings.map((warning) => (
          <li key={`${warning.code}:${warning.message}`}>
            <DashboardText>{warning.message}</DashboardText>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-1">
      <DashboardLabel>{label}</DashboardLabel>
      <DashboardText>{value}</DashboardText>
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
