import type {
  FactorySessionArtifactSummary,
  FactorySessionDispatchSummary,
} from "../../../api/factory-sessions/durable-inspection";
import type { components } from "../../../api/generated/openapi";
import {
  ButtonLink,
  DashboardLabel,
  DashboardText,
} from "../../../components/ui";
import { DashboardEmptyStateText } from "../../../components/ui/widget-frame";
import {
  buildProviderSessionDetailHref,
  formatProviderSessionRefLabel,
} from "../lib/provider-session-detail-href";
import type { FactorySessionDetailMessages } from "../messages/factory-session-detail";

export function DurableDispatchInspectionSection({
  dispatches,
  messages,
}: {
  dispatches: FactorySessionDispatchSummary[];
  messages: FactorySessionDetailMessages;
}) {
  return (
    <section aria-label={messages.dispatchesHeading} className="grid gap-2">
      <DashboardLabel as="h3">{messages.dispatchesHeading}</DashboardLabel>
      {dispatches.length === 0 ? (
        <DashboardEmptyStateText>
          {messages.dispatchesEmptyState}
        </DashboardEmptyStateText>
      ) : (
        <ul className="grid gap-3">
          {dispatches.map((dispatch) => (
            <li key={dispatch.id}>
              <DurableDispatchRow dispatch={dispatch} messages={messages} />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

export function DurableArtifactInspectionSection({
  artifacts,
  messages,
}: {
  artifacts: FactorySessionArtifactSummary[];
  messages: FactorySessionDetailMessages;
}) {
  return (
    <section aria-label={messages.artifactsHeading} className="grid gap-2">
      <DashboardLabel as="h3">{messages.artifactsHeading}</DashboardLabel>
      {artifacts.length === 0 ? (
        <DashboardEmptyStateText>
          {messages.artifactsEmptyState}
        </DashboardEmptyStateText>
      ) : (
        <ul className="grid gap-3">
          {artifacts.map((artifact) => (
            <li key={artifact.id}>
              <DurableArtifactRow artifact={artifact} messages={messages} />
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function DurableDispatchRow({
  dispatch,
  messages,
}: {
  dispatch: FactorySessionDispatchSummary;
  messages: FactorySessionDetailMessages;
}) {
  const summaryParts = [
    dispatch.id,
    dispatch.status,
    dispatch.dispatchKind,
    dispatch.label,
    dispatch.phase,
  ].filter((value): value is string => Boolean(value && value.trim() !== ""));

  return (
    <div className="grid gap-2 rounded-md border border-outline-variant p-3">
      <DashboardText>{summaryParts.join(" · ")}</DashboardText>
      {dispatch.outputArtifactIds && dispatch.outputArtifactIds.length > 0 ? (
        <Metric
          label={messages.dispatchOutputArtifactsLabel}
          value={dispatch.outputArtifactIds.join(", ")}
        />
      ) : null}
      {dispatch.providerSessionRefs &&
      dispatch.providerSessionRefs.length > 0 ? (
        <div className="grid gap-2">
          <DashboardLabel>{messages.providerSessionHeading}</DashboardLabel>
          <ul className="grid gap-2">
            {dispatch.providerSessionRefs.map((providerSessionRef) => (
              <li
                key={`${dispatch.id}:${providerSessionRef.provider}:${providerSessionRef.id}`}
              >
                <ProviderSessionInspectionLink
                  messages={messages}
                  providerSessionRef={providerSessionRef}
                />
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

function DurableArtifactRow({
  artifact,
  messages,
}: {
  artifact: FactorySessionArtifactSummary;
  messages: FactorySessionDetailMessages;
}) {
  const summaryParts = [artifact.id, artifact.kind];
  if (artifact.label) {
    summaryParts.push(artifact.label);
  }

  return (
    <div className="grid gap-2 rounded-md border border-outline-variant p-3">
      <DashboardText>{summaryParts.join(" · ")}</DashboardText>
      {artifact.dispatchId ? (
        <Metric
          label={messages.artifactDispatchLabel}
          value={artifact.dispatchId}
        />
      ) : null}
      {artifact.retrievalRef?.href ? (
        <div className="grid gap-1">
          <DashboardLabel>{messages.artifactRetrievalLabel}</DashboardLabel>
          <ButtonLink
            className="w-fit"
            href={artifact.retrievalRef.href}
            size="sm"
            tone="outline"
          >
            {messages.inspectArtifactAction}
          </ButtonLink>
        </div>
      ) : null}
    </div>
  );
}

function ProviderSessionInspectionLink({
  messages,
  providerSessionRef,
}: {
  messages: FactorySessionDetailMessages;
  providerSessionRef: components["schemas"]["LoadableProviderSessionRef"];
}) {
  const href = buildProviderSessionDetailHref(providerSessionRef);
  const label = formatProviderSessionRefLabel(providerSessionRef);

  if (!href) {
    return <DashboardText>{label}</DashboardText>;
  }

  return (
    <ButtonLink className="w-fit" href={href} size="sm" tone="outline">
      {messages.inspectProviderSessionAction}: {label}
    </ButtonLink>
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
