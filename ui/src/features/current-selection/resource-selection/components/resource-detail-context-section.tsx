import { DashboardLabel, DashboardText } from "../../../../components/ui";
import { formatList } from "../../../../components/ui/formatters";
import { CurrentSelectionSectionHeader } from "../../base/components/current-selection-section-header";
import { CurrentSelectionSupportingText } from "../../base/public";
import type { ResourceDetailState } from "../lib/detail-card-types";
import {
  resourceShowsModelFields,
  resourceShowsProviderQuotaFields,
} from "../lib/resource-detail-values";
import type { getResourceDetailMessages } from "../messages/resource-detail";

function SummaryField({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-0.5">
      <DashboardLabel>{label}</DashboardLabel>
      <DashboardText as="span" className="m-0 text-on-surface">
        {value}
      </DashboardText>
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
    <div className="mt-4 grid gap-4 [&_h4]:m-0">
      <section
        aria-labelledby="resource-summary-heading"
        className="grid gap-2.5"
      >
        <CurrentSelectionSectionHeader
          headingId="resource-summary-heading"
          title={messages.summaryHeading}
        />
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
      </section>

      <section
        aria-labelledby="resource-referencing-workers-heading"
        className="grid gap-2"
      >
        <CurrentSelectionSectionHeader
          headingId="resource-referencing-workers-heading"
          title={messages.referencingWorkersHeading}
        />
        {workerNames.length > 0 ? (
          <DashboardText className="m-0 text-on-surface">
            {formatList(workerNames)}
          </DashboardText>
        ) : (
          <CurrentSelectionSupportingText>
            {messages.referencingWorkersEmpty}
          </CurrentSelectionSupportingText>
        )}
      </section>

      <section
        aria-labelledby="resource-referencing-workstations-heading"
        className="grid gap-2"
      >
        <CurrentSelectionSectionHeader
          headingId="resource-referencing-workstations-heading"
          title={messages.referencingWorkstationsHeading}
        />
        {workstationNames.length > 0 ? (
          <DashboardText className="m-0 text-on-surface">
            {formatList(workstationNames)}
          </DashboardText>
        ) : (
          <CurrentSelectionSupportingText>
            {messages.referencingWorkstationsEmpty}
          </CurrentSelectionSupportingText>
        )}
      </section>
    </div>
  );
}
