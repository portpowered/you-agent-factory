import { Plus } from "lucide-react";
import type { ReactElement } from "react";

import { Button } from "../../../components/ui";

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
