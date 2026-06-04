import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import { useResourceDetailState } from "../hooks/use-resource-detail-state";
import type { ResourceDetailCardProps } from "../lib/detail-card-types";
import { getResourceDetailMessages } from "../messages/resource-detail";
import { ResourceDetailContextSection } from "./resource-detail-context-section";
import { ResourceEditableConfigurationSection } from "./resource-editable-configuration-section";

export function ResourceDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  resourceName,
  saveState,
  tokenCount = null,
  widgetId = "current-selection",
}: ResourceDetailCardProps) {
  const messages = getResourceDetailMessages(locale);
  const detailState = useResourceDetailState(resourceName);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      {editableConfigurationState?.status !== "ready" ? (
        <p className={WIDGET_SUBTITLE_CLASS}>{resourceName}</p>
      ) : null}
      {detailState.status === "loading" ? (
        <p
          className={cn(
            "m-0 text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
          {messages.configurationLoading}
        </p>
      ) : null}
      {detailState.status === "error" ? (
        <p
          className={cn(
            "m-0 text-on-error-container",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          role="alert"
        >
          {messages.configurationErrorPrefix} {detailState.errorMessage}
        </p>
      ) : null}
      {detailState.status === "empty" ? (
        <p
          className={cn(
            "m-0 text-on-surface-variant",
            DASHBOARD_BODY_TEXT_CLASS,
          )}
        >
          {messages.configurationEmpty}
        </p>
      ) : null}
      {detailState.status === "ready" && editableConfigurationState ? (
        <ResourceEditableConfigurationSection
          detailState={detailState}
          messages={messages}
          resourceName={resourceName}
          saveState={saveState}
          state={editableConfigurationState}
          tokenCount={tokenCount}
        />
      ) : null}
      {detailState.status === "ready" && !editableConfigurationState ? (
        <ResourceDetailContextSection
          detailState={detailState}
          messages={messages}
          tokenCount={tokenCount}
        />
      ) : null}
    </SelectionDetailLayout>
  );
}
