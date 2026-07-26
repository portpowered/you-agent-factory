import { surfacePanelVariants } from "@you-agent-factory/components/layout";
import type { FactoryGraphBulkSelectionSummary } from "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import { SelectionDetailLayout } from "../../base/components/layout/current-selection-detail-layout";
import { CurrentSelectionDescriptionList } from "../../base/components/detail/current-selection-description-list";
import { CurrentSelectionDetailItem } from "../../base/components/detail/current-selection-detail-item";
import { CurrentSelectionBodyLayout } from "../../base/components/layout/current-selection-body-layout";
import {
  getGraphBulkSelectionDetailMessages,
  graphBulkSelectionKindLabel,
} from "../messages/graph-bulk-selection-detail";

export interface GraphBulkSelectionDetailCardProps {
  locale?: string;
  summary: FactoryGraphBulkSelectionSummary;
  widgetId?: string;
}

export function GraphBulkSelectionDetailCard({
  locale,
  summary,
  widgetId = "current-selection",
}: GraphBulkSelectionDetailCardProps) {
  const messages = getGraphBulkSelectionDetailMessages(locale);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={messages.heading}>
        <section
          aria-label={messages.summaryRegionLabel}
          className={surfacePanelVariants({
            className: "grid gap-3",
          })}
        >
          <CurrentSelectionDescriptionList>
            <CurrentSelectionDetailItem
              label={messages.selectedItemCountLabel}
              value={messages.selectedItemCountValue(summary.totalCount)}
            />
            {summary.kindCounts.map(({ count, kind }) => (
              <CurrentSelectionDetailItem
                key={kind}
                label={graphBulkSelectionKindLabel(messages, kind)}
                value={String(count)}
              />
            ))}
          </CurrentSelectionDescriptionList>
        </section>
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
