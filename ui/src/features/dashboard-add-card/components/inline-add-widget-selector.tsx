import type { ChangeEvent, ReactElement } from "react";

import { Select } from "../../../components/ui";
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
  onChange: (event: ChangeEvent<HTMLSelectElement>) => void;
  options: InlineAddWidgetSelectorOption[];
  unavailableLabel: string;
  value: string;
}

export function InlineAddWidgetSelector({
  actionLabel,
  availabilityByType,
  disabled,
  onChange,
  options,
  unavailableLabel,
  value,
}: InlineAddWidgetSelectorProps): ReactElement {
  return (
    <Select
      aria-label={actionLabel}
      disabled={disabled}
      onChange={onChange}
      value={value}
    >
      {disabled ? (
        <option disabled value="">
          {unavailableLabel}
        </option>
      ) : null}
      {options.map((option) => {
        const availability = availabilityByType.get(option.widgetType);

        return (
          <option
            disabled={!availability?.enabled}
            key={option.widgetType}
            value={option.widgetType}
          >
            {option.title}
          </option>
        );
      })}
    </Select>
  );
}
