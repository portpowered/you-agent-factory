import {
  type ChangeEvent,
  type ReactElement,
  useEffect,
  useMemo,
  useState,
} from "react";

import { DashboardPanelShell } from "../../../components/ui/dashboard-shell";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../components/ui/dashboard-typography";
import { AgentBentoCardHeader } from "../../bento/components/agent-bento";
import type { DashboardWidgetPickerAvailability } from "../../bento/lib/dashboard-widget-picker";
import { getInlineAddWidgetMessages } from "../../bento/messages/inline-add-widget";
import { getInlineWidgetPickerOptions } from "../../bento/messages/inline-widget-picker";
import { InlineAddWidgetAddButton } from "./inline-add-widget-add-button";
import { InlineAddWidgetSelector } from "./inline-add-widget-selector";

export interface InlineAddWidgetCardProps {
  pickerAvailability?: DashboardWidgetPickerAvailability[];
  onSelectWidget?: (
    widgetType: DashboardWidgetPickerAvailability["widgetType"],
  ) => void;
  locale?: string;
}

export function InlineAddWidgetCard({
  locale,
  onSelectWidget,
  pickerAvailability = [],
}: InlineAddWidgetCardProps): ReactElement {
  const messages = getInlineAddWidgetMessages(locale);
  const options = useMemo(() => getInlineWidgetPickerOptions(locale), [locale]);
  const availabilityByType = useMemo(
    () => new Map(pickerAvailability.map((item) => [item.widgetType, item])),
    [pickerAvailability],
  );
  const firstEnabledWidgetType =
    pickerAvailability.find((item) => item.enabled)?.widgetType ?? "";
  const [selectedWidgetType, setSelectedWidgetType] = useState<string>(
    firstEnabledWidgetType,
  );
  const hasEnabledWidgets = pickerAvailability.some((item) => item.enabled);
  const selectedAvailability = selectedWidgetType
    ? availabilityByType.get(
        selectedWidgetType as DashboardWidgetPickerAvailability["widgetType"],
      )
    : undefined;
  const selectedOption = options.find(
    (option) => option.widgetType === selectedWidgetType,
  );

  useEffect(() => {
    if (!hasEnabledWidgets) {
      setSelectedWidgetType("");
      return;
    }

    if (!selectedAvailability?.enabled) {
      setSelectedWidgetType(firstEnabledWidgetType);
    }
  }, [
    firstEnabledWidgetType,
    hasEnabledWidgets,
    selectedAvailability?.enabled,
  ]);

  function handleWidgetSelection(event: ChangeEvent<HTMLSelectElement>) {
    setSelectedWidgetType(event.target.value);
  }

  function handleAddWidget() {
    if (!selectedAvailability?.enabled) {
      return;
    }

    onSelectWidget?.(selectedAvailability.widgetType);
  }

  return (
    <DashboardPanelShell
      aria-label={messages.title}
      as="article"
      className="grid h-full min-h-0 min-w-0 place-items-stretch overflow-hidden"
      shellKind="grid-card"
    >
      <div className="grid h-full min-h-0 grid-rows-[auto_1fr]">
        <AgentBentoCardHeader
          headerContent={
            <h3
              className={`m-0 [overflow-wrap:anywhere] ${DASHBOARD_SECTION_HEADING_CLASS}`}
            >
              {messages.title}
            </h3>
          }
          title={messages.title}
        />

        <div className="grid min-h-0 content-end p-3 sm:p-4">
          <div className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2">
            <InlineAddWidgetSelector
              actionLabel={messages.actionLabel}
              availabilityByType={availabilityByType}
              disabled={!hasEnabledWidgets}
              onChange={handleWidgetSelection}
              options={options}
              unavailableLabel={messages.actionUnavailableLabel}
              value={selectedWidgetType}
            />
            <InlineAddWidgetAddButton
              disabled={!selectedAvailability?.enabled}
              onClick={handleAddWidget}
              selectedWidgetTitle={selectedOption?.title}
              title={messages.title}
            />
          </div>
        </div>
      </div>
    </DashboardPanelShell>
  );
}
