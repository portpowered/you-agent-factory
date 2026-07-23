import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { NoSelectionDetailCardProps } from "../detail-card/detail-card-types";
import { SelectionDetailLayout } from "../layout/current-selection-detail-layout";
import { useCurrentSelectionShellMessages } from "../presentation/current-selection-locale";

export function NoSelectionDetailCard({
  widgetId = "current-selection",
}: NoSelectionDetailCardProps) {
  const messages = useCurrentSelectionShellMessages();

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <WidgetDetailCopy>{messages.emptyStateGuidance}</WidgetDetailCopy>
    </SelectionDetailLayout>
  );
}
