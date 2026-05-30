import {
  type ChangeEvent,
  type ReactElement,
  useEffect,
  useMemo,
  useState,
} from "react";

import { AgentBentoCard } from "../../bento/public";
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
    <AgentBentoCard
      bodyClassName="grid min-h-0 content-end"
      title={messages.title}
    >
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
    </AgentBentoCard>
  );
}
