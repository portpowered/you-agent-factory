import type { ReactElement } from "react";

import { cn } from "../../../lib/cn";
import { getAgentBentoMessages } from "../messages/agent-bento";

const REMOVE_BUTTON_CLASS = cn(
  "inline-grid size-8 shrink-0 place-items-center rounded-md border border-af-overlay/12 bg-transparent text-af-ink/54 outline-af-danger/45 transition-colors hover:border-af-danger/28 hover:bg-af-danger/10 hover:text-af-danger-ink focus-visible:outline-2 focus-visible:outline-offset-2",
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
