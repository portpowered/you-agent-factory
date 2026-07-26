import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { useState } from "react";
import type { components } from "../../../../api/generated/openapi";
import { ButtonLink, Heading, Label, Text } from "@you-agent-factory/components/primitives";
import { AlertPanel } from "../../../../components/ui/alert-panel";
import { ExpandablePanelTrigger } from "../../../../components/ui/expandable-panel-trigger";
import { WorkContentReadOnlyList } from "../../../work-content/components/work-content-read-only-list";
import { useFactorySessionArtifactDrilldown } from "../../hooks/use-factory-session-artifact-drilldown";
import { hasUsableArtifactDownload } from "../../lib/factory-session-artifact-drilldown";
import { getFactorySessionDetailMessages } from "../../messages/factory-session-detail";

type FactoryArtifact = components["schemas"]["FactoryArtifact"];

export function FactorySessionArtifactList({
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
      <Label>{heading}</Label>
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
          <Text>
            {artifact.kind}
            {artifact.label ? ` — ${artifact.label}` : ""}
          </Text>
          <Text variant="supporting">{artifact.id}</Text>
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
    return (
      <WidgetDetailCopy>{messages.artifactDetailLoadingState}</WidgetDetailCopy>
    );
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
      <Heading as="h4">{messages.artifactDetailHeading}</Heading>
      <div className="grid gap-2 sm:grid-cols-2">
        <Metric
          label={messages.artifactIdLabel}
          value={state.artifact.artifactId}
        />
        <Metric
          label={messages.artifactKindLabel}
          value={state.artifact.kind}
        />
        <Metric
          label={messages.artifactVisibilityLabel}
          value={state.artifact.visibility}
        />
        {state.artifact.label ? (
          <Metric
            label={messages.artifactLabelValueLabel}
            value={state.artifact.label}
          />
        ) : null}
        {state.artifact.dispatchId ? (
          <Metric
            label={messages.artifactDispatchIdLabel}
            value={state.artifact.dispatchId}
          />
        ) : null}
        {state.artifact.capture?.sourceDispatchId ? (
          <Metric
            label={messages.artifactSourceDispatchIdLabel}
            value={state.artifact.capture.sourceDispatchId}
          />
        ) : null}
        {state.artifact.summary ? (
          <Metric
            label={messages.artifactSummaryLabel}
            value={state.artifact.summary}
          />
        ) : null}
        {state.artifact.createdAt ? (
          <Metric
            label={messages.artifactCreatedAtLabel}
            value={state.artifact.createdAt}
          />
        ) : null}
        {state.artifact.capture?.capturedAt ? (
          <Metric
            label={messages.artifactCapturedAtLabel}
            value={state.artifact.capture.capturedAt}
          />
        ) : null}
        {state.artifact.auditMode ? (
          <Metric
            label={messages.artifactAuditModeLabel}
            value={state.artifact.auditMode}
          />
        ) : null}
        {state.artifact.capture?.mimeType ? (
          <Metric
            label={messages.artifactCaptureMimeTypeLabel}
            value={state.artifact.capture.mimeType}
          />
        ) : null}
        {typeof state.artifact.sizeBytes === "number" ? (
          <Metric
            label={messages.artifactSizeBytesLabel}
            value={String(state.artifact.sizeBytes)}
          />
        ) : null}
        {state.artifact.contentHash ? (
          <Metric
            label={messages.artifactContentHashLabel}
            value={state.artifact.contentHash}
          />
        ) : null}
      </div>
      <div className="grid gap-2">
        <Label>{messages.artifactPreviewHeading}</Label>
        {state.artifact.preview.kind === "inline" ? (
          <WorkContentReadOnlyList
            ariaLabel={messages.artifactPreviewHeading}
            content={state.artifact.preview.content}
            landmark={false}
            showHeading={false}
          />
        ) : state.artifact.preview.kind === "download" &&
          hasUsableArtifactDownload(state.artifact) ? (
          <div className="grid gap-2">
            <Text variant="supporting">{messages.artifactDownloadState}</Text>
            <ButtonLink
              className="w-fit"
              href={state.artifact.preview.contentRef.href}
              size="sm"
              tone="outline"
            >
              {messages.artifactDownloadActionLabel}
            </ButtonLink>
          </div>
        ) : state.artifact.preview.kind === "download" ? (
          <Text variant="supporting">
            {messages.artifactDownloadUnavailableState}
          </Text>
        ) : (
          <Text variant="supporting">
            {messages.artifactDetailUnavailableState}
          </Text>
        )}
      </div>
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
