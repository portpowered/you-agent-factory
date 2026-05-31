import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { useResourceDetailState } from "../hooks/use-resource-detail-state";
import type { ResourceDetailCardProps } from "../lib/detail-card-types";
import { getResourceDetailMessages } from "../messages/resource-detail";
import { ResourceDetailContextSection } from "./resource-detail-context-section";

export function ResourceDetailCard({
  locale,
  resourceName,
  tokenCount = null,
  widgetId = "current-selection",
}: ResourceDetailCardProps) {
  const messages = getResourceDetailMessages(locale);
  const detailState = useResourceDetailState(resourceName);

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <p className={WIDGET_SUBTITLE_CLASS}>{resourceName}</p>
      {detailState.status === "loading" ? (
        <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.configurationLoading}
        </p>
      ) : null}
      {detailState.status === "error" ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {messages.configurationErrorPrefix} {detailState.errorMessage}
        </p>
      ) : null}
      {detailState.status === "empty" ? (
        <p className={cn("m-0 text-af-text-muted", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.configurationEmpty}
        </p>
      ) : null}
      {detailState.status === "ready" ? (
        <ResourceDetailContextSection
          detailState={detailState}
          messages={messages}
          tokenCount={tokenCount}
        />
      ) : null}
    </SelectionDetailLayout>
  );
}
