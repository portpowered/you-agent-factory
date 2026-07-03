import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@you-agent-factory/components";
import type { FactoryGraphNodeKind } from "../../lib/draft/factory-graph-draft-types";
import { FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS } from "../../lib/work-state/factory-graph-node-class-visibility";
import { getFactoryGraphEditorMessages } from "../../messages/editor";
import { FactoryGraphEditorTooltipActionButton } from "../chrome/factory-graph-editor-tooltip-button";
import { FactoryGraphEditorMenuHeader } from "../menu/factory-graph-editor-menu-header";
import { FactoryGraphEditorMenuItemButton } from "../menu/factory-graph-editor-menu-item-button";
import { FactoryGraphEditorMenuItemCopy } from "../menu/factory-graph-editor-menu-item-copy";

export function FactoryGraphEditorHideShowMenu({
  hiddenNodeClasses,
  locale,
  onClearPreferences,
  onOpenChange,
  onToggleHiddenNodeClass,
  open,
  preferencesDirty = false,
  pressed,
}: {
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string;
  onClearPreferences?: () => void;
  onOpenChange?: (open: boolean) => void;
  onToggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  open: boolean;
  preferencesDirty?: boolean;
  pressed: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <Popover onOpenChange={onOpenChange} open={open}>
      <PopoverTrigger asChild>
        <FactoryGraphEditorTooltipActionButton
          aria-label={messages.toolbarHideShowLabel}
          aria-pressed={pressed}
          iconOnly
          placement="above"
          tooltip={messages.toolbarHideShowDescription}
          tone={open ? "secondary" : "outline"}
          type="button"
        >
          <HideShowIcon />
        </FactoryGraphEditorTooltipActionButton>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        aria-label={messages.toolbarHideShowMenuAriaLabel}
        avoidCollisions={false}
        // tailwind-exception: intrinsic-sizing
        className="grid max-h-[min(var(--radix-popover-content-available-height),calc(100vh-8rem))] gap-2 overflow-y-auto"
        side="top"
        sideOffset={12}
      >
        <FactoryGraphEditorMenuHeader
          description={messages.toolbarHideShowMenuDescription}
          title={messages.toolbarHideShowMenuTitle}
        />
        <div className="grid gap-1" role="menu">
          {FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS.map((kind) => {
            const visible = !hiddenNodeClasses.has(kind);
            const label = messages.kindLabel(kind);

            return (
              <FactoryGraphEditorMenuItemButton
                aria-checked={visible}
                aria-label={label}
                key={kind}
                onClick={() => onToggleHiddenNodeClass(kind)}
                role="menuitemcheckbox"
                type="button"
              >
                <span className="flex w-full items-center justify-between gap-3">
                  <FactoryGraphEditorMenuItemCopy
                    description={messages.nodeClassVisibilityDescription(kind)}
                    label={label}
                  />
                  {visible ? <MenuCheckIcon /> : null}
                </span>
              </FactoryGraphEditorMenuItemButton>
            );
          })}
        </div>
        {onClearPreferences ? (
          <FactoryGraphEditorMenuItemButton
            aria-label={messages.toolbarClearPreferencesLabel}
            disabled={!preferencesDirty}
            onClick={onClearPreferences}
            role="menuitem"
            type="button"
          >
            <FactoryGraphEditorMenuItemCopy
              label={messages.toolbarClearPreferencesLabel}
            />
          </FactoryGraphEditorMenuItemButton>
        ) : null}
      </PopoverContent>
    </Popover>
  );
}

function MenuCheckIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="16"
    >
      <path d="m5 12 4 4L19 6" />
    </svg>
  );
}

function HideShowIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="18"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.8"
      viewBox="0 0 24 24"
      width="18"
    >
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}
