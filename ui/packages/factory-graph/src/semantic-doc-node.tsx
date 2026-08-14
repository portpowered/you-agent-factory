import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import { GraphSemanticIcon } from "./semantic-icon.js";
import {
  type FactoryGraphNodeHandle,
  FactoryGraphNodeShell,
} from "./semantic-node-shell.js";
import {
  factoryGraphNodeHoverClassName,
  factoryGraphNodeSurfaceClassName,
  factoryGraphNodeVisualIconClassName,
  factoryGraphNodeWrappedTextClassName,
} from "./semantic-node-style.js";
import {
  type FactoryGraphVisualState,
  resolveFactoryGraphVisualState,
} from "./visual-state.js";

export interface FactoryGraphDocNodeData extends Record<string, unknown> {
  activeFlow?: boolean;
  displayLabel: string;
  focused?: boolean;
  factoryGraphNodeId?: string;
  fileType?: string;
  handles: FactoryGraphNodeHandle[];
  kind: "doc";
  locale?: string;
  onSelectDoc?: (targetPath: string) => void;
  selectedDoc: boolean;
  targetPath: string;
  validationError?: boolean;
  muted?: boolean;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
}

export type FactoryGraphDocNode = Node<FactoryGraphDocNodeData, "doc">;

/** Original Factory document node, with host-owned selection callback. */
export function FactoryGraphDocNodeView({
  data,
  selected: reactFlowSelected,
}: NodeProps<FactoryGraphDocNode>) {
  const selectable = data.onSelectDoc !== undefined;
  const docLabel = "Document";
  const selected = data.selectedDoc || reactFlowSelected;
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    family: "doc",
    focused: data.focused,
    muted: data.muted,
    selected,
    validation: data.validationError,
  });

  return (
    <FactoryGraphNodeShell
      className={[
        factoryGraphNodeSurfaceClassName("neutral"),
        "justify-center text-left text-on-surface",
        factoryGraphNodeHoverClassName({
          activeFlow: data.activeFlow,
          muted: data.muted,
          selected,
          validationError: data.validationError,
        }),
      ]
        .filter(Boolean)
        .join(" ")}
      handles={data.handles}
      nodeType="doc"
      resizeControls={
        data.resizeControls
          ? { ...data.resizeControls, isVisible: selected }
          : undefined
      }
      visualState={{
        activeFlow: data.activeFlow,
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
      }}
    >
      {selectable ? (
        <GraphNodeButton
          aria-label={selectDocLabel(data.displayLabel, data.locale)}
          aria-invalid={data.validationError ? true : undefined}
          aria-pressed={selected}
          className="grid min-w-0 gap-0.5 overflow-hidden"
          data-selected-doc={selected ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectDoc?.(data.targetPath);
          }}
        >
          <FactoryGraphDocNodeContent
            displayLabel={data.displayLabel}
            docLabel={docLabel}
            targetPath={data.targetPath}
            visualState={visualState}
          />
        </GraphNodeButton>
      ) : (
        <FactoryGraphDocNodeContent
          displayLabel={data.displayLabel}
          docLabel={docLabel}
          targetPath={data.targetPath}
          visualState={visualState}
        />
      )}
    </FactoryGraphNodeShell>
  );
}

function FactoryGraphDocNodeContent({
  displayLabel,
  docLabel,
  targetPath,
  visualState,
}: {
  displayLabel: string;
  docLabel: string;
  targetPath: string;
  visualState: FactoryGraphVisualState;
}) {
  return (
    <div className="grid min-w-0 gap-1 px-2 py-1">
      <div className="flex min-w-0 items-center gap-1.5">
        <GraphSemanticIcon
          className={factoryGraphNodeVisualIconClassName(
            visualState,
            "text-on-surface-variant",
          )}
          kind="doc"
          label={docLabel}
        />
        <span
          className={factoryGraphNodeWrappedTextClassName(
            "block text-sm font-medium text-on-surface",
          )}
        >
          {displayLabel}
        </span>
      </div>
      <span
        className={factoryGraphNodeWrappedTextClassName(
          "block text-xs text-on-surface-variant",
        )}
      >
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
