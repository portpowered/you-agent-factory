import { Button } from "@you-agent-factory/components/primitives";
import { Plus } from "lucide-react";
import type { ReactElement } from "react";

export interface InlineAddWidgetAddButtonProps {
  disabled: boolean;
  onClick: () => void;
  selectedWidgetTitle?: string;
  title: string;
}

export function InlineAddWidgetAddButton({
  disabled,
  onClick,
  selectedWidgetTitle,
  title,
}: InlineAddWidgetAddButtonProps): ReactElement {
  return (
    <Button
      aria-label={
        selectedWidgetTitle ? `${title}: ${selectedWidgetTitle}` : title
      }
      data-dashboard-add-widget-control="true"
      disabled={disabled}
      onClick={onClick}
      size="icon"
      tone="outline"
      type="button"
    >
      <Plus aria-hidden="true" className="size-5" focusable="false" />
    </Button>
  );
}
