import type { Node, NodeProps } from "@xyflow/react";

import { cn } from "../../../lib/cn";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import { currentActivityGraphNodeHoverClassName } from "../lib/current-activity-graph-hover";
import { getActivityGraphMessages } from "../messages/activity-graph";
import { activityGraphNodeSurfaceClassName } from "./current-activity-node-chrome";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface DocNodeData extends Record<string, unknown> {
  displayLabel: string;
  factoryGraphNodeId?: string;
  fileType?: string;
  handles: ActivityGraphNodeHandle[];
  kind: "doc";
  locale?: string;
  onSelectDoc?: (targetPath: string) => void;
  selectedDoc: boolean;
  targetPath: string;
}

export type CurrentActivityDocNode = Node<DocNodeData, "doc">;

export function DocNodeView({ data }: NodeProps<CurrentActivityDocNode>) {
  const messages = getWorkflowActivityShellMessages(data.locale);
  const activityGraphMessages = getActivityGraphMessages(data.locale);
  const docLabel = activityGraphMessages.graphSemanticIconLabel("doc");
  const selectable = data.onSelectDoc !== undefined;

  return (
    <ActivityGraphNodeShell
      className={cn(
        activityGraphNodeSurfaceClassName("neutral"),
        "justify-center text-left text-on-surface",
        currentActivityGraphNodeHoverClassName({
          activeFlow: false,
          muted: false,
          selected: data.selectedDoc,
        }),
        data.selectedDoc && "border-primary shadow-af-accent-selected",
      )}
      handles={data.handles}
      nodeType="doc"
    >
      {selectable ? (
        <GraphNodeButton
          aria-label={messages.selectDocLabel(data.displayLabel)}
          aria-pressed={data.selectedDoc}
          className="grid min-w-0 gap-0.5 overflow-hidden"
          data-selected-doc={data.selectedDoc ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectDoc?.(data.targetPath);
          }}
        >
          <DocNodeContent
            displayLabel={data.displayLabel}
            docLabel={docLabel}
            locale={data.locale}
            targetPath={data.targetPath}
          />
        </GraphNodeButton>
      ) : (
        <DocNodeContent
          displayLabel={data.displayLabel}
          docLabel={docLabel}
          locale={data.locale}
          targetPath={data.targetPath}
        />
      )}
    </ActivityGraphNodeShell>
  );
}

function DocNodeContent({
  displayLabel,
  docLabel,
  locale,
  targetPath,
}: {
  displayLabel: string;
  docLabel: string;
  locale?: string;
  targetPath: string;
}) {
  return (
    <div className="grid min-w-0 gap-1 px-2 py-1">
      <div className="flex min-w-0 items-center gap-1.5">
        <GraphSemanticIcon
          className="text-on-surface-variant"
          kind="doc"
          label={docLabel}
          locale={locale}
        />
        <span className="truncate text-sm font-medium text-on-surface">
          {displayLabel}
        </span>
      </div>
      <span className="truncate text-xs text-on-surface-variant">
        {targetPath}
      </span>
    </div>
  );
}
