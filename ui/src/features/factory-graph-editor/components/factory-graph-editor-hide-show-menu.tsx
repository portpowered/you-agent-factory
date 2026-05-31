import { DashboardActionButton, Popover, PopoverContent, PopoverTrigger } from "../../../components/ui";
import type { FactoryGraphNodeKind } from "../lib/factory-graph-draft-types";
import { FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS } from "../lib/factory-graph-node-class-visibility";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import { FactoryGraphEditorTooltipActionButton } from "./factory-graph-editor-tooltip-button";

export function FactoryGraphEditorHideShowMenu({
  hiddenNodeClasses,
  locale,
  onOpenChange,
  onToggleHiddenNodeClass,
  open,
  pressed,
}: {
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  locale?: string;
  onOpenChange?: (open: boolean) => void;
  onToggleHiddenNodeClass: (kind: FactoryGraphNodeKind) => void;
  open: boolean;
  pressed: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);

  return (
    <Popover onOpenChange={onOpenChange} open={open}>
      <PopoverTrigger asChild>
        <FactoryGraphEditorTooltipActionButton
          aria-label={messages.toolbarOpenHideShowMenuLabel}
          aria-pressed={pressed}
          iconOnly
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
        <div className="grid gap-1">
          <p className="m-0 text-sm font-semibold text-af-text">
            {messages.toolbarHideShowMenuTitle}
          </p>
          <p className="m-0 text-xs leading-5 text-af-text-muted">
            {messages.toolbarHideShowMenuDescription}
          </p>
        </div>
        <div className="grid gap-1" role="menu">
          {FACTORY_GRAPH_TOGGLEABLE_NODE_KINDS.map((kind) => {
            const visible = !hiddenNodeClasses.has(kind);
            const label = messages.kindLabel(kind);

            return (
              <DashboardActionButton
                aria-checked={visible}
                aria-label={label}
                className="min-h-0 w-full justify-start rounded-2xl border-transparent px-3 py-2 text-left [&>span]:grid [&>span]:w-full [&>span]:justify-items-start"
                key={kind}
                onClick={() => onToggleHiddenNodeClass(kind)}
                role="menuitemcheckbox"
                tone="ghost"
                type="button"
              >
                <span className="flex w-full items-center justify-between gap-3">
                  <span className="grid justify-items-start gap-0.5">
                    <span className="text-sm font-semibold text-af-text">{label}</span>
                    <span className="text-xs leading-5 text-af-text-muted">
                      {messages.nodeClassVisibilityDescription(kind)}
                    </span>
                  </span>
                  {visible ? <MenuCheckIcon /> : null}
                </span>
              </DashboardActionButton>
            );
          })}
        </div>
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
