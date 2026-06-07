import { DetailCopy } from "../../../../../components/ui/widget-frame";
import type { NoSelectionDetailCardProps } from "../detail-card/detail-card-types";
import { SelectionDetailLayout } from "../layout/current-selection-detail-layout";
import { useCurrentSelectionShellMessages } from "../presentation/current-selection-locale";

export function NoSelectionDetailCard({
  widgetId = "current-selection",
}: NoSelectionDetailCardProps) {
  const messages = useCurrentSelectionShellMessages();

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <DetailCopy>{messages.emptyStateGuidance}</DetailCopy>
    </SelectionDetailLayout>
  );
}
