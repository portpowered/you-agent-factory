import { surfacePanelVariants } from "../../../../components/ui";
import type { FactoryGraphBulkSelectionSummary } from "../../../factory-graph-editor/lib/selection/factory-graph-bulk-selection-summary";
import { SelectionDetailLayout } from "../../base/components/layout/current-selection-detail-layout";
import {
  CurrentSelectionBodyLayout,
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailItem,
} from "../../base/public";
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
