import type { HTMLAttributes, ReactNode } from "react";

import { AgentBentoCard } from "../../../bento/components/agent-bento";

interface WorkflowActivityBentoShellProps {
  children: ReactNode;
  headerAction: ReactNode;
  title: string;
}

export function WorkflowActivityBentoShell({
  children,
  headerAction,
  title,
}: WorkflowActivityBentoShellProps) {
  return (
    <AgentBentoCard
      bodyClassName="h-full max-h-full min-h-0 overflow-hidden"
      bodyProps={
        {
          "data-workflow-activity-graph-body": "",
          style: { height: "100%", maxHeight: "100%", overflow: "hidden" },
        } as HTMLAttributes<HTMLDivElement>
      }
      bodyScroll={false}
      chromeDensity="compact"
      className="h-full max-h-full min-h-0 overflow-hidden"
      headerAction={headerAction}
      style={{ height: "100%", maxHeight: "100%", overflow: "hidden" }}
      title={title}
    >
      {children}
    </AgentBentoCard>
  );
}
