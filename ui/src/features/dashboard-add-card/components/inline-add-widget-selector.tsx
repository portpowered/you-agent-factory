import { EnumSelect } from "@you-agent-factory/components/forms";
import type { ReactElement } from "react";
import type {
  DashboardWidgetPickerAvailability,
  DashboardWidgetPickerWidgetType,
} from "../../bento/lib/dashboard-widget-picker";

export interface InlineAddWidgetSelectorOption {
  title: string;
  widgetType: DashboardWidgetPickerWidgetType;
}

export interface InlineAddWidgetSelectorProps {
  actionLabel: string;
  availabilityByType: ReadonlyMap<
    DashboardWidgetPickerWidgetType,
    DashboardWidgetPickerAvailability
  >;
  disabled: boolean;
  onValueChange: (value: string) => void;
  options: InlineAddWidgetSelectorOption[];
  unavailableLabel: string;
  value: string;
}

const INLINE_ADD_WIDGET_SELECTOR_ID = "inline-add-widget-selector";

export function InlineAddWidgetSelector({
  actionLabel,
  availabilityByType,
  disabled,
  onValueChange,
  options,
  unavailableLabel,
  value,
}: InlineAddWidgetSelectorProps): ReactElement {
  const selectorOptions = disabled
    ? [
        {
          disabled: true,
          label: unavailableLabel,
          value: unavailableLabel,
        },
      ]
    : options.map((option) => ({
        disabled: !availabilityByType.get(option.widgetType)?.enabled,
        label: option.title,
        value: option.widgetType,
      }));

  return (
    <EnumSelect
      aria-label={actionLabel}
      disabled={disabled}
      id={INLINE_ADD_WIDGET_SELECTOR_ID}
      onValueChange={onValueChange}
      options={selectorOptions}
      placeholder={disabled ? unavailableLabel : undefined}
      value={disabled ? unavailableLabel : value}
    />
  );
}
