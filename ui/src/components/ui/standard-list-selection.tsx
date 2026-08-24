import {
  createContext,
  forwardRef,
  type HTMLAttributes,
  type ReactNode,
  useContext,
} from "react";

import { cn } from "../../lib/cn";
import { DASHBOARD_BODY_TEXT_CLASS } from "./dashboard-typography";
import {
  SelectableCardButton,
  type SelectableCardButtonProps,
} from "./selectable-card-button";

export type StandardListSelectionTone =
  | "neutral"
  | "accent"
  | "success"
  | "danger";

const StandardListSelectionDisabledContext = createContext(false);

const STANDARD_LIST_SELECTION_LIST_CLASS = "grid gap-2";

const STANDARD_LIST_SELECTION_ROW_BASE_CLASS =
  "h-auto min-h-0 w-full justify-start gap-1 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-af-focus-ring";

const STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS =
  "border-outline bg-surface-container-high text-on-surface hover:border-outline-variant hover:bg-af-overlay";

const STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS =
  "border-primary bg-primary-container text-on-primary factory-light:text-on-primary-container";

const STANDARD_LIST_SELECTION_ROW_SUCCESS_CLASS =
  "border-af-success-border bg-success-container text-on-success-container";

const STANDARD_LIST_SELECTION_ROW_DANGER_CLASS =
  "border-af-danger-border bg-error-container text-on-error";

const STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS =
  "border-outline-variant bg-surface-container-low text-on-surface";

const STANDARD_LIST_SELECTION_TONE_CLASS: Record<
  StandardListSelectionTone,
  string
> = {
  accent: STANDARD_LIST_SELECTION_ROW_ACCENT_CLASS,
  danger: STANDARD_LIST_SELECTION_ROW_DANGER_CLASS,
  neutral: STANDARD_LIST_SELECTION_ROW_NEUTRAL_CLASS,
  success: STANDARD_LIST_SELECTION_ROW_SUCCESS_CLASS,
};

export function standardListSelectionRowClassName({
  className,
  selected,
  tone = "neutral",
}: {
  className?: string;
  selected: boolean;
  tone?: StandardListSelectionTone;
}): string {
  return cn(
    STANDARD_LIST_SELECTION_ROW_BASE_CLASS,
    selected
      ? STANDARD_LIST_SELECTION_ROW_SELECTED_CLASS
      : STANDARD_LIST_SELECTION_TONE_CLASS[tone],
    className,
  );
}

export interface StandardListSelectionProps
  extends HTMLAttributes<HTMLDivElement> {
  children: ReactNode;
  disabled?: boolean;
  selectionAnnouncement?: string;
}

export function StandardListSelection({
  children,
  className,
  disabled = false,
  selectionAnnouncement,
  ...props
}: StandardListSelectionProps) {
  return (
    <StandardListSelectionDisabledContext.Provider value={disabled}>
      <div
        aria-busy={disabled ? true : undefined}
        className={cn(STANDARD_LIST_SELECTION_LIST_CLASS, className)}
        {...props}
      >
        {children}
        {selectionAnnouncement ? (
          <div aria-live="polite" className="sr-only">
            {selectionAnnouncement}
          </div>
        ) : null}
      </div>
    </StandardListSelectionDisabledContext.Provider>
  );
}

export interface StandardListSelectionItemProps
  extends Omit<SelectableCardButtonProps, "selected" | "tone"> {
  selected?: boolean;
  textRole?: "body" | "none";
  tone?: StandardListSelectionTone;
}

export const StandardListSelectionItem = forwardRef<
  HTMLButtonElement,
  StandardListSelectionItemProps
>(function StandardListSelectionItem(
  {
    children,
    className,
    disabled,
    selected = false,
    textRole = "body",
    tone = "neutral",
    ...props
  },
  ref,
) {
  const listDisabled = useContext(StandardListSelectionDisabledContext);

  return (
    <SelectableCardButton
      className={standardListSelectionRowClassName({
        className: cn(
          textRole === "body" && DASHBOARD_BODY_TEXT_CLASS,
          className,
        ),
        selected,
        tone,
      })}
      data-selected={selected ? "true" : "false"}
      disabled={disabled ?? listDisabled}
      ref={ref}
      selected={selected}
      size="sm"
      tone="outline"
      {...props}
    >
      {children}
    </SelectableCardButton>
  );
});
