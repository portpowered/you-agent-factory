import { factoryBundledDocDisplayLabel } from "../../../workflow-activity/lib/factory-bundled-docs";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import {
  CurrentSelectionBodyLayout,
  CurrentSelectionDetailFeedback,
} from "../../base/public";
import { useDocDetailState } from "../hooks/use-doc-detail-state";
import { getDocDetailMessages } from "../messages/doc-detail";

export function DocDetailCard({
  locale,
  targetPath,
  widgetId = "current-selection",
}: {
  locale?: string;
  targetPath: string;
  widgetId?: string;
}) {
  const messages = getDocDetailMessages(locale);
  const detailState = useDocDetailState(targetPath, locale);
  const title =
    detailState.status === "ready"
      ? detailState.displayLabel
      : factoryBundledDocDisplayLabel(targetPath);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={title}>
        {detailState.status === "loading" ? (
          <CurrentSelectionDetailFeedback>
            {messages.configurationLoading}
          </CurrentSelectionDetailFeedback>
        ) : null}
        {detailState.status === "error" ? (
          <CurrentSelectionDetailFeedback role="alert" tone="danger">
            {messages.configurationErrorPrefix} {detailState.errorMessage}
          </CurrentSelectionDetailFeedback>
        ) : null}
        {detailState.status === "empty" ? (
          <CurrentSelectionDetailFeedback>
            {messages.configurationEmpty}
          </CurrentSelectionDetailFeedback>
        ) : null}
        {detailState.status === "ready" ? (
          <CurrentSelectionDetailFeedback>
            {messages.docKindLabel}: {detailState.targetPath}
          </CurrentSelectionDetailFeedback>
        ) : null}
      </CurrentSelectionBodyLayout>
    </SelectionDetailLayout>
  );
}
