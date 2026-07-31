import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";

import { GraphSemanticIcon } from "./semantic-icon.js";
import { FactoryGraphNodeShell, type FactoryGraphNodeHandle } from "./semantic-node-shell.js";
import {
  factoryGraphNodeHoverClassName,
  factoryGraphNodeSurfaceClassName,
} from "./semantic-node-style.js";

export interface FactoryGraphDocNodeData extends Record<string, unknown> {
  displayLabel: string;
  factoryGraphNodeId?: string;
  fileType?: string;
  handles: FactoryGraphNodeHandle[];
  kind: "doc";
  locale?: string;
  onSelectDoc?: (targetPath: string) => void;
  selectedDoc: boolean;
  targetPath: string;
}

export type FactoryGraphDocNode = Node<FactoryGraphDocNodeData, "doc">;

/** Original Factory document node, with host-owned selection callback. */
export function FactoryGraphDocNodeView({
  data,
}: NodeProps<FactoryGraphDocNode>) {
  const selectable = data.onSelectDoc !== undefined;
  const docLabel = "Document";

  return (
    <FactoryGraphNodeShell
      className={[
        factoryGraphNodeSurfaceClassName("neutral"),
        "justify-center text-left text-on-surface",
        factoryGraphNodeHoverClassName({ selected: data.selectedDoc }),
        data.selectedDoc && "border-primary shadow-af-accent-selected",
      ]
        .filter(Boolean)
        .join(" ")}
      handles={data.handles}
      nodeType="doc"
    >
      {selectable ? (
        <GraphNodeButton
          aria-label={selectDocLabel(data.displayLabel, data.locale)}
          aria-pressed={data.selectedDoc}
          className="grid min-w-0 gap-0.5 overflow-hidden"
          data-selected-doc={data.selectedDoc ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectDoc?.(data.targetPath);
          }}
        >
          <FactoryGraphDocNodeContent
            displayLabel={data.displayLabel}
            docLabel={docLabel}
            targetPath={data.targetPath}
          />
        </GraphNodeButton>
      ) : (
        <FactoryGraphDocNodeContent
          displayLabel={data.displayLabel}
          docLabel={docLabel}
          targetPath={data.targetPath}
        />
      )}
    </FactoryGraphNodeShell>
  );
}

function FactoryGraphDocNodeContent({
  displayLabel,
  docLabel,
  targetPath,
}: {
  displayLabel: string;
  docLabel: string;
  targetPath: string;
}) {
  return (
    <div className="grid min-w-0 gap-1 px-2 py-1">
      <div className="flex min-w-0 items-center gap-1.5">
        <GraphSemanticIcon
          className="text-on-surface-variant"
          kind="doc"
          label={docLabel}
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

function selectDocLabel(displayLabel: string, locale?: string): string {
  return locale === "zh-CN"
    ? `选择 ${displayLabel} 文档`
    : `Select ${displayLabel} doc`;
}
