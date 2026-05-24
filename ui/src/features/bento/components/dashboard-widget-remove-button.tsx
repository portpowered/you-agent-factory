import type { ReactElement } from "react";

import { cn } from "../../../lib/cn";
import { getAgentBentoMessages } from "../messages/agent-bento";

const REMOVE_BUTTON_CLASS = cn(
  "inline-grid size-8 shrink-0 place-items-center rounded-md border border-af-border bg-transparent text-af-text-subtle transition-colors hover:border-af-danger-border hover:bg-af-danger-surface hover:text-af-danger-text focus-visible:ring-2 focus-visible:ring-af-focus-ring focus-visible:ring-offset-0",
);

export interface DashboardWidgetRemoveButtonProps {
  locale?: string | null;
  onClick?: () => void;
  widgetTitle: string;
}

export function DashboardWidgetRemoveButton({
  locale,
  onClick,
  widgetTitle,
}: DashboardWidgetRemoveButtonProps): ReactElement {
  const messages = getAgentBentoMessages(locale);

  return (
    <button
      aria-label={messages.removeWidgetLabel(widgetTitle)}
      className={REMOVE_BUTTON_CLASS}
      onClick={onClick}
      type="button"
    >
      <svg
        aria-hidden="true"
        fill="none"
        height="16"
        viewBox="0 0 16 16"
        width="16"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          d="m4 4 8 8M12 4 4 12"
          stroke="currentColor"
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth="1.7"
        />
      </svg>
    </button>
  );
}
