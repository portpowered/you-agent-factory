import type { components } from "../../../api/generated/openapi";
import { isDurableFactorySessionID } from "../../../api/factory-sessions";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import {
  AlertPanel,
  DashboardHeading,
  DashboardLabel,
  DashboardText,
} from "../../../components/ui";
import { DetailCopy } from "../../../components/ui/widget-frame";
import {
  DurableFactorySessionDetailError,
  DurableFactorySessionDetailLoading,
  DurableFactorySessionDetailNotFound,
  DurableFactorySessionDetailSuccess,
} from "./durable-factory-session-detail-states";
import type { FactorySessionDetailData } from "../hooks/use-factory-session-detail";
import { useFactorySessionDetail } from "../hooks/use-factory-session-detail";
import { getFactorySessionDetailMessages } from "../messages/factory-session-detail";

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
  const isDurableSession =
    sessionID !== null &&
    sessionID.trim() !== "" &&
    isDurableFactorySessionID(sessionID);

  if (sessionID === null || sessionID.trim() === "") {
    return null;
  }

  return (
    <section aria-label={messages.selectedSessionHeading} className="grid gap-4">
      <DashboardHeading>{messages.selectedSessionHeading}</DashboardHeading>
      <div className="grid gap-2">
        <DashboardLabel>{messages.sessionIdLabel}</DashboardLabel>
        <DashboardText>{sessionID}</DashboardText>
      </div>

      {detailState.status === "loading" ? (
        isDurableSession ? (
          <DurableFactorySessionDetailLoading messages={messages} />
        ) : (
          <DetailCopy>{messages.loadingState}</DetailCopy>
        )
      ) : null}
      {detailState.status === "not-found" ? (
        isDurableSession ? (
          <DurableFactorySessionDetailNotFound messages={messages} />
        ) : (
          <DetailCopy>{messages.missingState}</DetailCopy>
        )
      ) : null}
      {detailState.status === "error" ? (
        isDurableSession ? (
          <DurableFactorySessionDetailError
            message={detailState.message}
            messages={messages}
          />
        ) : (
          <AlertPanel tone="danger">
            {detailState.message ?? messages.errorState}
          </AlertPanel>
        )
      ) : null}
      {detailState.status === "success" ? (
        detailState.data.kind === "durable" ? (
          <DurableFactorySessionDetailSuccess
            data={detailState.data}
            messages={messages}
          />
        ) : (
          <FactorySessionRuntimeSections
            data={detailState.data}
            locale={locale}
          />
        )
      ) : null}
    </section>
  );
}

function FactorySessionRuntimeSections({
  data,
  locale,
}: {
  data: Extract<FactorySessionDetailData, { kind: "live" }>;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const runtime = data.session.runtime;

  return (
    <div className="grid gap-4">
      <div className="grid gap-2 sm:grid-cols-2">
        <Metric label={messages.orchestratorKindLabel} value={runtime.orchestratorKind} />
        <Metric label={messages.statusLabel} value={runtime.status} />
      </div>

      {runtime.orchestratorKind === FactoryOrchestratorKind.JAVASCRIPT ? (
        <JavaScriptSessionProjection
          artifacts={runtime.artifacts}
          dispatches={runtime.dispatches}
          javascript={runtime.javascript}
          locale={locale}
          partialResult={data.partialResult}
          result={data.result}
        />
      ) : (
        <PetriSessionProjection locale={locale} petri={runtime.petri} />
      )}
    </div>
  );
}

function JavaScriptSessionProjection({
  artifacts,
  dispatches,
  javascript,
  locale,
  partialResult,
  result,
}: {
  artifacts?: FactoryArtifact[];
  dispatches?: FactoryDispatch[];
  javascript?: components["schemas"]["FactorySessionJavaScriptProjection"];
  locale?: string;
  partialResult?: components["schemas"]["FactorySessionPartialResult"];
  result?: components["schemas"]["FactorySessionLiveResult"];
}) {
  const messages = getFactorySessionDetailMessages(locale);

  if (!javascript) {
    return <DetailCopy>{messages.dynamicWorkflowShorthand}</DetailCopy>;
  }

  const warnings = (dispatches ?? []).flatMap(
    (dispatch) => dispatch.warnings ?? [],
  );

  return (
    <div className="grid gap-4">
      <DetailCopy>{messages.dynamicWorkflowShorthand}</DetailCopy>
      <div className="grid gap-2 sm:grid-cols-2">
        {javascript.phase ? (
          <Metric label={messages.phaseLabel} value={javascript.phase} />
        ) : null}
        <Metric
          label={messages.scriptStatusLabel}
          value={javascript.scriptStatus}
        />
        <Metric
          label={messages.childDispatchCountsLabel}
          value={`queued ${javascript.childDispatchCounts.queued}, running ${javascript.childDispatchCounts.running}, completed ${javascript.childDispatchCounts.completed}`}
        />
      </div>
      {javascript.phases.length > 0 ? (
        <Metric label={messages.phasesLabel} value={javascript.phases.join(", ")} />
      ) : null}

      {javascript.checkpoints && javascript.checkpoints.length > 0 ? (
        <CheckpointRefList
          checkpoints={javascript.checkpoints}
          heading={messages.checkpointRefsHeading}
        />
      ) : null}

      {artifacts && artifacts.length > 0 ? (
        <ArtifactList artifacts={artifacts} heading={messages.artifactsHeading} />
      ) : null}

      {warnings.length > 0 ? (
        <WarningList heading={messages.warningsHeading} warnings={warnings} />
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
              {checkpoint.label ? `${checkpoint.id} (${checkpoint.label})` : checkpoint.id}
              {checkpoint.summary ? ` — ${checkpoint.summary}` : ""}
            </DashboardText>
          </li>
        ))}
      </ul>
    </div>
  );
}

function ArtifactList({
  artifacts,
  heading,
}: {
  artifacts: FactoryArtifact[];
  heading: string;
}) {
  return (
    <div className="grid gap-2">
      <DashboardLabel>{heading}</DashboardLabel>
      <ul className="grid gap-1">
        {artifacts.map((artifact) => (
          <li key={artifact.id}>
            <DashboardText>
              {artifact.kind}
              {artifact.label ? ` — ${artifact.label}` : ""}
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

function formatArtifactRef(
  artifactRef: components["schemas"]["FactoryArtifactRef"],
): string {
  return `${artifactRef.id} · ${artifactRef.kind}`;
}
