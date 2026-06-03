import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { WIDGET_SUBTITLE_CLASS } from "../../../../components/ui/widget-frame";
import { cn } from "../../../../lib/cn";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import type { WorkTypeDetailCardProps } from "../lib/detail-card-types";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";
import { WorkTypeReadySection } from "./work-type-ready-section";

export function WorkTypeDetailCard({
  editableConfigurationState,
  headerAction,
  locale,
  onSelectWorkStateGraphNode,
  saveState,
  widgetId = "current-selection",
  workTypeName,
}: WorkTypeDetailCardProps) {
  const messages = getWorkTypeDetailMessages(locale);

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      {editableConfigurationState?.status !== "ready" ? (
        <p className={WIDGET_SUBTITLE_CLASS}>{workTypeName}</p>
      ) : null}
      {editableConfigurationState?.status === "loading" ? (
        <p className={cn("m-0 text-on-surface-variant", DASHBOARD_BODY_TEXT_CLASS)}>
          {messages.configurationLoading}
        </p>
      ) : null}
      {editableConfigurationState?.status === "error" ? (
        <p
          className={cn("m-0 text-on-error-container", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {messages.configurationErrorPrefix}{" "}
          {editableConfigurationState.errorMessage}
        </p>
      ) : null}
      {editableConfigurationState?.status === "empty" ? (
        <p className={cn("m-0 text-on-surface-variant", DASHBOARD_BODY_TEXT_CLASS)}>
          {editableConfigurationState.message ?? messages.configurationEmpty}
        </p>
      ) : null}
      {editableConfigurationState?.status === "ready" ? (
        <WorkTypeReadySection
          messages={messages}
          onSelectWorkStateGraphNode={onSelectWorkStateGraphNode}
          saveState={saveState}
          state={editableConfigurationState}
          workTypeName={workTypeName}
        />
      ) : null}
    </SelectionDetailLayout>
  );
}
