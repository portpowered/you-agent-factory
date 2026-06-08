import type { ReactNode } from "react";

import { DashboardActionRow, DashboardText } from "../../../../components/ui";

export interface CurrentSelectionWorkRowProps {
  actions?: ReactNode;
  status?: ReactNode;
  supportingContent?: ReactNode;
  title: ReactNode;
}

export function CurrentSelectionWorkRow({
  actions,
  status,
  supportingContent,
  title,
}: CurrentSelectionWorkRowProps) {
  return (
    <DashboardText as="li" className="grid min-w-0 gap-2 rounded-lg px-3 py-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <strong className="min-w-0 flex-1 [overflow-wrap:anywhere]">
          {title}
        </strong>
        <DashboardActionRow
          actions={actions}
          actionsClassName="justify-end"
          className="justify-end"
          statuses={status}
          statusesClassName="justify-end"
        />
      </div>
      {supportingContent}
    </DashboardText>
  );
}
