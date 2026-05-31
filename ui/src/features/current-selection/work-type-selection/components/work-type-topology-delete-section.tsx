import type { ReactNode } from "react";
import { Trash2 } from "lucide-react";

import { DashboardActionButton } from "../../../../components/ui";
import { DASHBOARD_BODY_TEXT_CLASS } from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import { factoryGraphNodeIdForWorkType } from "../../../factory-graph-editor/lib/factory-validation-graph-projection";
import { useFactoryGraphTopologyEditorBridge } from "../../../workflow-activity/state/factory-graph-topology-editor-bridge";
import { CurrentSelectionSectionHeader } from "../../base/components/detail-card-shared";
import type { WorkTypeDetailMessages } from "../messages/work-type-detail-types";

export function WorkTypeTopologyDeleteSection({
  messages,
  workTypeName,
}: {
  messages: WorkTypeDetailMessages;
  workTypeName: string;
}) {
  const topologyEditor = useFactoryGraphTopologyEditorBridge(
    (state) => state.handlers,
  );
  if (!topologyEditor?.editorMode || !topologyEditor.canInteractWithEditor) {
    return null;
  }

  const nodeId = factoryGraphNodeIdForWorkType(workTypeName);
  const blockedReason =
    topologyEditor.blockedRemovalReason &&
    topologyEditor.blockedRemovalReason.length > 0
      ? topologyEditor.blockedRemovalReason
      : null;
  const sectionId = `work-type-topology-delete-${workTypeName}`;

  return (
    <section
      aria-labelledby={sectionId}
      className="mt-4 grid gap-2.5 [&_h4]:m-0"
    >
      <CurrentSelectionSectionHeader
        headingId={sectionId}
        title={messages.topologyDeleteHeading}
      />
      <DashboardActionRow
        actions={
          <DashboardActionButton
            aria-label={messages.topologyDeleteAction(workTypeName)}
            onClick={() => {
              topologyEditor.requestNodeRemoval(nodeId);
            }}
            tone="destructive"
            type="button"
          >
            <Trash2 aria-hidden="true" className="size-4" />
          </DashboardActionButton>
        }
      />
      {blockedReason ? (
        <p
          className={cn("m-0 text-af-danger-text", DASHBOARD_BODY_TEXT_CLASS)}
          role="alert"
        >
          {messages.topologyDeleteBlockedPrefix} {blockedReason}
        </p>
      ) : null}
    </section>
  );
}

function DashboardActionRow({
  actions,
}: {
  actions: ReactNode;
}) {
  return <div className="flex justify-end">{actions}</div>;
}
