import { useId } from "react";

import { Label, Text } from "../../../../components/ui";
import { formatList } from "../../../../components/ui/formatters";
import {
  CurrentSelectionExpandableSection,
  CurrentSelectionSupportingText,
} from "../../base/public";
import type { ResourceDetailState } from "../lib/detail-card-types";
import {
  resourceShowsModelFields,
  resourceShowsProviderQuotaFields,
} from "../lib/resource-detail-values";
import type { getResourceDetailMessages } from "../messages/resource-detail";

function SummaryField({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-0.5">
      <Label>{label}</Label>
      <Text as="span" className="m-0 text-on-surface">
        {value}
      </Text>
    </div>
  );
}

export function ResourceDetailContextSection({
  detailState,
  messages,
  tokenCount,
}: {
  detailState: Extract<ResourceDetailState, { status: "ready" }>;
  messages: ReturnType<typeof getResourceDetailMessages>;
  tokenCount?: number | null;
}) {
  const { resource, workerNames, workstationNames } = detailState;
  const typeLabel = resource.type
    ? messages.localizeResourceType(resource.type)
    : messages.notConfiguredValue;

  return (
    <>
      <ResourceSummarySection
        messages={messages}
        resource={resource}
        tokenCount={tokenCount}
        typeLabel={typeLabel}
      />
      <ResourceReferencingWorkersSection
        messages={messages}
        workerNames={workerNames}
      />
      <ResourceReferencingWorkstationsSection
        messages={messages}
        workstationNames={workstationNames}
      />
    </>
  );
}

export function ResourceSummarySection({
  messages,
  resource,
  tokenCount,
  typeLabel,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  resource: Extract<ResourceDetailState, { status: "ready" }>["resource"];
  tokenCount?: number | null;
  typeLabel: string;
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.summaryHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      <div className="grid gap-2">
        <SummaryField label={messages.nameFieldLabel} value={resource.name} />
        <SummaryField
          label={messages.capacityFieldLabel}
          value={String(resource.capacity)}
        />
        <SummaryField label={messages.typeFieldLabel} value={typeLabel} />
        {tokenCount !== null && tokenCount !== undefined ? (
          <SummaryField
            label={messages.tokenCountFieldLabel}
            value={String(tokenCount)}
          />
        ) : null}
        {resourceShowsModelFields(resource) && resource.model ? (
          <SummaryField
            label={messages.modelFieldLabel}
            value={resource.model}
          />
        ) : null}
        {resourceShowsModelFields(resource) && resource.backend ? (
          <SummaryField
            label={messages.backendFieldLabel}
            value={resource.backend}
          />
        ) : null}
        {resourceShowsModelFields(resource) && resource.loadPolicy ? (
          <SummaryField
            label={messages.loadPolicyFieldLabel}
            value={resource.loadPolicy}
          />
        ) : null}
        {resourceShowsProviderQuotaFields(resource) && resource.provider ? (
          <SummaryField
            label={messages.providerFieldLabel}
            value={resource.provider}
          />
        ) : null}
      </div>
    </CurrentSelectionExpandableSection>
  );
}

export function ResourceReferencingWorkersSection({
  messages,
  workerNames,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  workerNames: string[];
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.referencingWorkersHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {workerNames.length > 0 ? (
        <Text className="m-0 text-on-surface">{formatList(workerNames)}</Text>
      ) : (
        <CurrentSelectionSupportingText>
          {messages.referencingWorkersEmpty}
        </CurrentSelectionSupportingText>
      )}
    </CurrentSelectionExpandableSection>
  );
}

export function ResourceReferencingWorkstationsSection({
  messages,
  workstationNames,
}: {
  messages: ReturnType<typeof getResourceDetailMessages>;
  workstationNames: string[];
}) {
  const sectionId = useId();
  const contentId = `${sectionId}-content`;
  const headingId = `${sectionId}-heading`;

  return (
    <CurrentSelectionExpandableSection
      contentId={contentId}
      defaultExpanded
      headingId={headingId}
      title={messages.referencingWorkstationsHeading}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {workstationNames.length > 0 ? (
        <Text className="m-0 text-on-surface">
          {formatList(workstationNames)}
        </Text>
      ) : (
        <CurrentSelectionSupportingText>
          {messages.referencingWorkstationsEmpty}
        </CurrentSelectionSupportingText>
      )}
    </CurrentSelectionExpandableSection>
  );
}
