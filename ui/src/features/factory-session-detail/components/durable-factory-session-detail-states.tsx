import type { components } from "../../../api/generated/openapi";
import {
  AlertPanel,
  DashboardLabel,
  DashboardText,
} from "../../../components/ui";
import {
  DashboardEmptyState,
  DashboardEmptyStateText,
  DashboardEmptyStateTitle,
} from "../../../components/ui/widget-frame";
import type { FactorySessionDetailData } from "../hooks/use-factory-session-detail";
import {
  resolveDurableInspectionPresentation,
  resolveDurableResultStatus,
} from "../lib/durable-inspection-state";
import type { FactorySessionDetailMessages } from "../messages/factory-session-detail";

type DurableDetailData = Extract<FactorySessionDetailData, { kind: "durable" }>;

export function DurableFactorySessionDetailLoading({
  messages,
}: {
  messages: FactorySessionDetailMessages;
}) {
  return (
    <DashboardEmptyState
      aria-busy="true"
      aria-label={messages.durableDetailRegionLabel}
      aria-live="polite"
      compact
      role="status"
    >
      <DashboardEmptyStateTitle>
        {messages.durableLoadingTitle}
      </DashboardEmptyStateTitle>
      <DashboardEmptyStateText>{messages.durableLoadingState}</DashboardEmptyStateText>
    </DashboardEmptyState>
  );
}

export function DurableFactorySessionDetailNotFound({
  messages,
}: {
  messages: FactorySessionDetailMessages;
}) {
  return (
    <DashboardEmptyState
      aria-label={messages.durableDetailRegionLabel}
      aria-live="polite"
      compact
      role="status"
    >
      <DashboardEmptyStateTitle>
        {messages.durableMissingTitle}
      </DashboardEmptyStateTitle>
      <DashboardEmptyStateText>{messages.durableMissingState}</DashboardEmptyStateText>
    </DashboardEmptyState>
  );
}

export function DurableFactorySessionDetailError({
  message,
  messages,
}: {
  message?: string;
  messages: FactorySessionDetailMessages;
}) {
  return (
    <AlertPanel aria-live="assertive" role="alert" tone="danger">
      <DashboardEmptyStateTitle as="h3">
        {messages.durableErrorTitle}
      </DashboardEmptyStateTitle>
      <DashboardEmptyStateText>
        {message ?? messages.durableErrorState}
      </DashboardEmptyStateText>
    </AlertPanel>
  );
}

export function DurableFactorySessionDetailSuccess({
  data,
  messages,
}: {
  data: DurableDetailData;
  messages: FactorySessionDetailMessages;
}) {
  const presentation = resolveDurableInspectionPresentation(data);

  if (presentation === "terminal") {
    return (
      <DurableFactorySessionTerminalSections data={data} messages={messages} />
    );
  }

  return (
    <DurableFactorySessionPartialSections data={data} messages={messages} />
  );
}

function DurableFactorySessionPartialSections({
  data,
  messages,
}: {
  data: DurableDetailData;
  messages: FactorySessionDetailMessages;
}) {
  const session = data.session;
  const partialResult = data.durablePartialResult;
  const resultStatus = resolveDurableResultStatus(data);

  return (
    <div className="grid gap-4">
      <DashboardEmptyState
        aria-label={messages.durableDetailRegionLabel}
        compact
        role="status"
      >
        <DashboardEmptyStateTitle>
          {messages.durablePartialTitle}
        </DashboardEmptyStateTitle>
        <DashboardEmptyStateText>
          {messages.durablePartialState}
        </DashboardEmptyStateText>
      </DashboardEmptyState>

      <DurableSessionSummaryFields messages={messages} session={session} />

      {resultStatus ? (
        <Metric
          label={messages.durableResultStatusLabel}
          value={resultStatus}
        />
      ) : null}

      {partialResult?.availability?.message ? (
        <Metric
          label={messages.durableAvailabilityLabel}
          value={partialResult.availability.message}
        />
      ) : null}

      {session.resultSummary?.summary ? (
        <Metric
          label={messages.resultSummaryLabel}
          value={session.resultSummary.summary}
        />
      ) : null}

      {partialResult?.artifactRefs && partialResult.artifactRefs.length > 0 ? (
        <ArtifactRefList
          artifactRefs={partialResult.artifactRefs}
          heading={messages.durablePartialArtifactRefsHeading}
        />
      ) : null}
    </div>
  );
}

function DurableFactorySessionTerminalSections({
  data,
  messages,
}: {
  data: DurableDetailData;
  messages: FactorySessionDetailMessages;
}) {
  const session = data.session;
  const durableResult = data.durableResult ?? data.durablePartialResult;
  const resultStatus = resolveDurableResultStatus(data);
  const failureMessage =
    durableResult?.failure?.message ?? session.failure?.message;

  return (
    <div className="grid gap-4">
      <DashboardEmptyState
        aria-label={messages.durableDetailRegionLabel}
        compact
        role="status"
      >
        <DashboardEmptyStateTitle>
          {messages.durableTerminalTitle}
        </DashboardEmptyStateTitle>
        <DashboardEmptyStateText>
          {messages.durableTerminalState}
        </DashboardEmptyStateText>
      </DashboardEmptyState>

      <DurableSessionSummaryFields messages={messages} session={session} />

      {resultStatus ? (
        <Metric
          label={messages.durableResultStatusLabel}
          value={resultStatus}
        />
      ) : null}

      {session.resultSummary?.summary ? (
        <Metric
          label={messages.resultSummaryLabel}
          value={session.resultSummary.summary}
        />
      ) : null}

      {failureMessage ? (
        <Metric label={messages.durableFailureLabel} value={failureMessage} />
      ) : null}

      {durableResult?.artifactRefs && durableResult.artifactRefs.length > 0 ? (
        <ArtifactRefList
          artifactRefs={durableResult.artifactRefs}
          heading={messages.durableTerminalArtifactRefsHeading}
        />
      ) : null}
    </div>
  );
}

function DurableSessionSummaryFields({
  messages,
  session,
}: {
  messages: FactorySessionDetailMessages;
  session: components["schemas"]["FactorySessionDurableReadModel"];
}) {
  const progress = session.progress;
  const progressSummary =
    progress === undefined
      ? undefined
      : [
          progress.completedDispatches !== undefined
            ? `completed ${progress.completedDispatches}`
            : null,
          progress.inFlightDispatches !== undefined
            ? `in flight ${progress.inFlightDispatches}`
            : null,
          progress.totalDispatches !== undefined
            ? `total ${progress.totalDispatches}`
            : null,
        ]
          .filter((value): value is string => value !== null)
          .join(", ");

  return (
    <div className="grid gap-4">
      <div className="grid gap-2 sm:grid-cols-2">
        <Metric
          label={messages.orchestratorKindLabel}
          value={session.orchestratorKind}
        />
        <Metric label={messages.statusLabel} value={session.status} />
      </div>

      {session.phase ? (
        <Metric label={messages.phaseLabel} value={session.phase} />
      ) : null}

      {session.resolvedSource.sourceRef ? (
        <Metric
          label={messages.resolvedSourceLabel}
          value={session.resolvedSource.sourceRef}
        />
      ) : null}

      {progressSummary ? (
        <Metric label={messages.progressLabel} value={progressSummary} />
      ) : null}
    </div>
  );
}

function ArtifactRefList({
  artifactRefs,
  heading,
}: {
  artifactRefs: components["schemas"]["FactoryArtifactRef"][];
  heading: string;
}) {
  return (
    <div className="grid gap-2">
      <DashboardLabel>{heading}</DashboardLabel>
      <ul className="grid gap-1">
        {artifactRefs.map((artifactRef) => (
          <li key={artifactRef.id}>
            <DashboardText>
              {artifactRef.id} · {artifactRef.kind}
            </DashboardText>
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
