import type { ReactElement } from "react";

import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { getAgentBentoMessages } from "../messages/agent-bento";

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
    <DashboardActionButton
      aria-label={messages.removeWidgetLabel(widgetTitle)}
      className="size-8 rounded-md"
      iconOnly
      onClick={onClick}
      tone="destructive"
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
    </DashboardActionButton>
  );
}
