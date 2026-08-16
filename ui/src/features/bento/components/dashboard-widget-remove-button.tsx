import type { ReactElement } from "react";

import { DashboardActionButton } from "../../../components/ui/dashboard-action-button";
import { getAgentBentoMessages } from "../messages/agent-bento";

export interface DashboardWidgetRemoveButtonProps {
  locale?: string | null;
  onClick?: () => void;
  widgetInstanceID?: string;
  widgetTitle: string;
}

export function DashboardWidgetRemoveButton({
  locale,
  onClick,
  widgetInstanceID,
  widgetTitle,
}: DashboardWidgetRemoveButtonProps): ReactElement {
  const messages = getAgentBentoMessages(locale);

  return (
    <DashboardActionButton
      aria-label={messages.removeWidgetLabel(widgetTitle)}
      data-dashboard-widget-remove="true"
      data-dashboard-widget-instance-id={widgetInstanceID}
      iconOnly
      onClick={onClick}
      tone="outline"
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
