import type { ReactNode } from "react";
import {
  ButtonLink,
  DashboardStatusPill,
  DescriptionList,
  Label,
  Text,
} from "../../../../components/ui";
import type { FactorySessionDispatchDrilldownModel } from "../../lib/factory-session-dispatch-detail";
import { getFactorySessionDetailMessages } from "../../messages/factory-session-detail";
import { resolveFactoryDispatchStatusTone } from "../../messages/factory-session-runtime-display";

export function DispatchDetailContent({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <div className="grid gap-4">
      <DispatchSummaryMetrics data={data} locale={locale} />
      <DispatchJavaScriptSection data={data} locale={locale} />
      <DispatchPetriSection data={data} locale={locale} />
      <DispatchStatusHistorySection data={data} locale={locale} />
      <DispatchProviderSessionsSection data={data} locale={locale} />
      <DispatchArtifactLinksSection data={data} locale={locale} />
      <DispatchRelatedWorkSection data={data} locale={locale} />
      <DispatchFailureSection data={data} locale={locale} />
      <DispatchUsageSection data={data} locale={locale} />
      {data.warnings.length > 0 ? (
        <DispatchDetailSection heading={messages.warningsHeading}>
          <DispatchDetailList
            items={data.warnings}
            keyForItem={(warning) => `${warning.code}:${warning.message}`}
            renderItem={(warning) => (
              <DescriptionList>
                <DispatchDetailItem
                  code
                  label={messages.warningCodeLabel}
                  value={warning.code}
                />
                <DispatchDetailItem
                  label={messages.failureMessageLabel}
                  value={warning.message}
                />
              </DescriptionList>
            )}
          />
        </DispatchDetailSection>
      ) : null}
    </div>
  );
}

function DispatchSummaryMetrics({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);

  return (
    <div className="grid gap-2 sm:grid-cols-2">
      <Metric label={messages.dispatchKindLabel} value={data.dispatchKind} />
      <div className="grid gap-1">
        <Label>{messages.dispatchStatusLabel}</Label>
        <DashboardStatusPill
          size="compact"
          tone={resolveFactoryDispatchStatusTone({
            status: data.status,
            warningCount: data.warnings.length,
          })}
        >
          {data.status}
        </DashboardStatusPill>
      </div>
      {data.label ? (
        <Metric label={messages.dispatchLabelField} value={data.label} />
      ) : null}
      {data.attempt ? (
        <Metric
          label={messages.dispatchAttemptLabel}
          value={String(data.attempt)}
        />
      ) : null}
      {data.phase ? (
        <Metric label={messages.phaseLabel} value={data.phase} />
      ) : null}
      {data.runnerID ? (
        <Metric label={messages.runnerIdLabel} value={data.runnerID} />
      ) : null}
      {data.model ? (
        <Metric label={messages.modelLabel} value={data.model} />
      ) : null}
      {data.provider ? (
        <Metric label={messages.providerLabel} value={data.provider} />
      ) : null}
      {data.promptDigest ? (
        <Metric label={messages.promptDigestLabel} value={data.promptDigest} />
      ) : null}
      {data.schemaDigest ? (
        <Metric label={messages.schemaDigestLabel} value={data.schemaDigest} />
      ) : null}
    </div>
  );
}

function DispatchJavaScriptSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (!data.javascript) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.javascriptTaskHeading}>
      <DescriptionList>
        <DispatchDetailItem
          label={messages.javascriptTaskKindLabel}
          value={data.javascript.taskKind}
        />
        <DispatchDetailItem
          label={messages.javascriptTaskLabel}
          value={data.javascript.taskLabel}
        />
        <DispatchDetailItem
          label={messages.executionModeLabel}
          value={data.javascript.executionMode}
        />
      </DescriptionList>
    </DispatchDetailSection>
  );
}

function DispatchPetriSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (!data.petri) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.petriDetailHeading}>
      <DescriptionList>
        <DispatchDetailItem
          label={messages.petriTransitionLabel}
          value={data.petri.transitionId}
        />
        <DispatchDetailItem
          label={messages.petriWorkerTypeLabel}
          value={data.petri.workerType}
        />
        <DispatchDetailItem
          label={messages.petriWorkstationLabel}
          value={data.petri.workstationName}
        />
      </DescriptionList>
    </DispatchDetailSection>
  );
}

function DispatchStatusHistorySection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (data.statusHistory.length === 0) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.statusHistoryHeading}>
      <DispatchDetailList
        items={data.statusHistory}
        keyForItem={(status) => status}
        renderItem={(status) => <Text>{status}</Text>}
      />
    </DispatchDetailSection>
  );
}

function DispatchProviderSessionsSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (data.providerSessionRefs.length === 0) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.providerSessionHeading}>
      <DispatchDetailList
        items={data.providerSessionRefs}
        keyForItem={(ref) => `${ref.provider}:${ref.kind}:${ref.id}`}
        renderItem={(ref) => (
          <DescriptionList>
            <DispatchDetailItem
              label={messages.providerLabel}
              value={ref.provider}
            />
            <DispatchDetailItem
              label={messages.providerSessionRefLabel}
              value={`${ref.kind} · ${ref.id}`}
            />
          </DescriptionList>
        )}
      />
    </DispatchDetailSection>
  );
}

function DispatchArtifactLinksSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (data.artifactLinks.length === 0) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.artifactLinksHeading}>
      <DispatchDetailList
        items={data.artifactLinks}
        keyForItem={(artifact) => artifact.id}
        renderItem={(artifact) => (
          <ButtonLink
            className="w-fit"
            href={artifact.href}
            size="sm"
            title={messages.artifactRefActionLabel}
            tone="outline"
          >
            {artifact.id}
          </ButtonLink>
        )}
      />
    </DispatchDetailSection>
  );
}

function DispatchRelatedWorkSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (data.relatedWorkIDs.length === 0) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.relatedWorkHeading}>
      <DispatchDetailList
        items={data.relatedWorkIDs}
        keyForItem={(workID) => workID}
        renderItem={(workID) => <Text>{workID}</Text>}
      />
    </DispatchDetailSection>
  );
}

function DispatchFailureSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (!data.failureDetail) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.failureDetailHeading}>
      <DescriptionList aria-label={messages.failureDetailHeading}>
        <DispatchDetailItem
          code
          label={messages.failureReasonLabel}
          value={data.failureDetail.reason}
        />
        <DispatchDetailItem
          label={messages.failureMessageLabel}
          value={data.failureDetail.message}
        />
      </DescriptionList>
    </DispatchDetailSection>
  );
}

function DispatchUsageSection({
  data,
  locale,
}: {
  data: FactorySessionDispatchDrilldownModel;
  locale?: string;
}) {
  const messages = getFactorySessionDetailMessages(locale);
  if (!data.usage) {
    return null;
  }

  return (
    <DispatchDetailSection heading={messages.usageHeading}>
      <DescriptionList>
        <DispatchDetailItem
          label={messages.usageInputTokensLabel}
          value={formatNumericDetail(data.usage.inputTokens)}
        />
        <DispatchDetailItem
          label={messages.usageOutputTokensLabel}
          value={formatNumericDetail(data.usage.outputTokens)}
        />
        <DispatchDetailItem
          label={messages.usageTotalTokensLabel}
          value={formatNumericDetail(data.usage.totalTokens)}
        />
        <DispatchDetailItem
          label={messages.usageRetryCountLabel}
          value={formatNumericDetail(data.usage.retryCount)}
        />
        <DispatchDetailItem
          label={messages.usageDurationLabel}
          value={formatDurationMillis(data.usage.durationMillis)}
        />
        <DispatchDetailItem
          label={messages.usageCostLabel}
          value={formatCostUsd(data.usage.costUsd)}
        />
      </DescriptionList>
    </DispatchDetailSection>
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

function DispatchDetailSection({
  children,
  heading,
}: {
  children: ReactNode;
  heading: string;
}) {
  return (
    <section className="grid gap-2 border-t border-outline pt-3">
      <Label>{heading}</Label>
      {children}
    </section>
  );
}

function DispatchDetailItem({
  code = false,
  label,
  value,
}: {
  code?: boolean;
  label: string;
  value?: string;
}) {
  if (!value) {
    return null;
  }

  return (
    <div>
      <dt className="text-on-surface-subtle">{label}</dt>
      <dd>
        {code ? (
          <code className="rounded-sm bg-surface-container px-1 py-0.5 text-[0.95em]">
            {value}
          </code>
        ) : (
          value
        )}
      </dd>
    </div>
  );
}

function DispatchDetailList<T>({
  items,
  keyForItem,
  renderItem,
}: {
  items: T[];
  keyForItem: (item: T) => string;
  renderItem: (item: T) => ReactNode;
}) {
  return (
    <ul className="grid gap-2">
      {items.map((item) => (
        <li
          className="rounded-md border border-outline bg-surface-container-low px-3 py-2"
          key={keyForItem(item)}
        >
          {renderItem(item)}
        </li>
      ))}
    </ul>
  );
}

function formatNumericDetail(value: number | undefined): string | undefined {
  return typeof value === "number" ? value.toLocaleString("en-US") : undefined;
}

function formatDurationMillis(value: number | undefined): string | undefined {
  return typeof value === "number"
    ? `${value.toLocaleString("en-US")} ms`
    : undefined;
}

function formatCostUsd(value: number | undefined): string | undefined {
  return typeof value === "number" ? `$${value.toFixed(2)}` : undefined;
}
