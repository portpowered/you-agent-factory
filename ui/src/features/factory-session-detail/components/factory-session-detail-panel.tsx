import type { components } from "../../../api/generated/openapi";
import { FactoryOrchestratorKind } from "../../../api/generated/openapi";
import { useState } from "react";
import {
  AlertPanel,
  ButtonLink,
  DashboardHeading,
  DashboardLabel,
  DashboardText,
  ExpandablePanelTrigger,
} from "../../../components/ui";
import { DetailCopy } from "../../../components/ui/widget-frame";
import { WorkContentReadOnlyList } from "../../work-content/public";
import { useFactorySessionArtifactDrilldown } from "../hooks/use-factory-session-artifact-drilldown";
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
        <DetailCopy>{messages.loadingState}</DetailCopy>
      ) : null}
      {detailState.status === "not-found" ? (
        <DetailCopy>{messages.missingState}</DetailCopy>
      ) : null}
      {detailState.status === "error" ? (
        <AlertPanel tone="danger">
          {detailState.message ?? messages.errorState}
        </AlertPanel>
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
  dispatches,
  javascript,
  locale,
  partialResult,
  result,
  sessionID,
}: {
  artifacts?: FactoryArtifact[];
  dispatches?: FactoryDispatch[];
  javascript?: components["schemas"]["FactorySessionJavaScriptProjection"];
  locale?: string;
  partialResult?: components["schemas"]["FactorySessionPartialResult"];
  result?: components["schemas"]["FactorySessionLiveResult"];
  sessionID: string;
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
        <ArtifactList
          artifacts={artifacts}
          heading={messages.artifactsHeading}
          locale={locale}
          sessionID={sessionID}
        />
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
  locale,
  sessionID,
}: {
  artifacts: FactoryArtifact[];
  heading: string;
  locale?: string;
  sessionID: string | null;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const [expandedArtifactID, setExpandedArtifactID] = useState<string | null>(
    null,
  );

  return (
    <div className="grid gap-2">
      <DashboardLabel>{heading}</DashboardLabel>
      <ul className="grid gap-1">
        {artifacts.map((artifact) => (
          <li key={artifact.id}>
            <ArtifactDisclosure
              artifact={artifact}
              expanded={expandedArtifactID === artifact.id}
              locale={locale}
              messages={messages}
              onToggle={(expanded) => {
                setExpandedArtifactID(expanded ? artifact.id : null);
              }}
              sessionID={sessionID}
            />
          </li>
        ))}
      </ul>
    </div>
  );
}

function ArtifactDisclosure({
  artifact,
  expanded,
  locale,
  messages,
  onToggle,
  sessionID,
}: {
  artifact: FactoryArtifact;
  expanded: boolean;
  locale?: string;
  messages: ReturnType<typeof getFactorySessionDetailMessages>;
  onToggle: (expanded: boolean) => void;
  sessionID: string | null;
}) {
  const contentID = `factory-session-artifact-${artifact.id}`;

  return (
    <div className="grid gap-2 rounded-lg border border-outline bg-surface-container-low p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="grid gap-1">
          <DashboardText>
            {artifact.kind}
            {artifact.label ? ` — ${artifact.label}` : ""}
          </DashboardText>
          <DashboardText variant="supporting">{artifact.id}</DashboardText>
        </div>
        <ExpandablePanelTrigger
          aria-label={`${messages.artifactViewLabel} ${artifact.id}`}
          controlsID={contentID}
          expanded={expanded}
          onClick={() => {
            onToggle(!expanded);
          }}
          variant="compact"
        >
          {messages.artifactViewLabel}
        </ExpandablePanelTrigger>
      </div>
      {expanded ? (
        <div className="grid gap-3 border-t border-outline pt-3" id={contentID}>
          <ArtifactDrilldownBody
            artifactID={artifact.id}
            locale={locale}
            sessionID={sessionID}
          />
        </div>
      ) : null}
    </div>
  );
}

function ArtifactDrilldownBody({
  artifactID,
  locale,
  sessionID,
}: {
  artifactID: string;
  locale?: string;
  sessionID: string | null;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  const state = useFactorySessionArtifactDrilldown(sessionID, artifactID, true);

  if (state.status === "loading") {
    return <DetailCopy>{messages.artifactDetailLoadingState}</DetailCopy>;
  }

  if (state.status === "error") {
    return (
      <AlertPanel tone="danger">
        {state.failure.message || messages.artifactDetailErrorState}
      </AlertPanel>
    );
  }

  if (state.status !== "success") {
    return null;
  }

  return (
    <div className="grid gap-3">
      <DashboardHeading as="h4">{messages.artifactDetailHeading}</DashboardHeading>
      <div className="grid gap-2 sm:grid-cols-2">
        <Metric label={messages.artifactIdLabel} value={state.artifact.artifactId} />
        <Metric
          label={messages.artifactVisibilityLabel}
          value={state.artifact.visibility}
        />
        {state.artifact.dispatchId ? (
          <Metric
            label={messages.artifactDispatchIdLabel}
            value={state.artifact.dispatchId}
          />
        ) : null}
        {state.artifact.summary ? (
          <Metric
            label={messages.artifactSummaryLabel}
            value={state.artifact.summary}
          />
        ) : null}
      </div>
      <div className="grid gap-2">
        <DashboardLabel>{messages.artifactPreviewHeading}</DashboardLabel>
        {state.artifact.preview.kind === "inline" ? (
          <WorkContentReadOnlyList
            ariaLabel={messages.artifactPreviewHeading}
            content={state.artifact.preview.content}
            landmark={false}
            showHeading={false}
          />
        ) : state.artifact.preview.kind === "download" ? (
          <div className="grid gap-2">
            <DashboardText variant="supporting">
              {messages.artifactDetailUnavailableState}
            </DashboardText>
            <ButtonLink
              className="w-fit"
              href={state.artifact.preview.contentRef.href}
              size="sm"
              tone="outline"
            >
              {messages.artifactViewLabel}
            </ButtonLink>
          </div>
        ) : (
          <DashboardText variant="supporting">
            {messages.artifactDetailUnavailableState}
          </DashboardText>
        )}
      </div>
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
