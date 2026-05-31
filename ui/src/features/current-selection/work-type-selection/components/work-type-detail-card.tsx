import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeTopologyDeleteSection } from "./work-type-topology-delete-section";

export function WorkTypeDetailCard({
  locale,
  widgetId = "current-selection",
  workTypeName,
}: {
  locale?: string | null;
  widgetId?: string;
  workTypeName: string;
}) {
  const messages = getWorkTypeDetailMessages(locale);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{workTypeName}</p>
      <WorkTypeTopologyDeleteSection
        messages={messages}
        workTypeName={workTypeName}
      />
    </SelectionDetailLayout>
  );
}
