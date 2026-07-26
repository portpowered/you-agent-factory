import type { ReactNode } from "react";

import { ActionRow } from "@you-agent-factory/components/layout";
import { Text } from "@you-agent-factory/components/primitives";

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
    <Text as="li" className="grid min-w-0 gap-2 rounded-lg px-3 py-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <strong className="min-w-0 flex-1 [overflow-wrap:anywhere]">
          {title}
        </strong>
        <ActionRow
          actions={actions}
          actionsClassName="justify-end"
          className="justify-end"
          statuses={status}
          statusesClassName="justify-end"
        />
      </div>
      {supportingContent}
    </Text>
  );
}
