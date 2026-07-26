import { forwardRef, type ReactNode } from "react";

import { SelectableCardButton } from "../../../../../components/ui/selectable-card-button";
import { cn } from "../../../../../lib/cn";

type CurrentSelectionSelectableButtonVariant = "card" | "compact";
type CurrentSelectionSelectableButtonSelectionStyle = "accent" | "outline";

export interface CurrentSelectionSelectableButtonProps {
  "aria-label"?: string;
  children: ReactNode;
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  selectedStyle?: CurrentSelectionSelectableButtonSelectionStyle;
  type?: "button" | "submit" | "reset";
  variant?: CurrentSelectionSelectableButtonVariant;
}

export const CurrentSelectionSelectableButton = forwardRef<
  HTMLButtonElement,
  CurrentSelectionSelectableButtonProps
>(function CurrentSelectionSelectableButton(
  {
    children,
    className,
    onClick,
    selected = false,
    selectedStyle = "accent",
    type = "button",
    variant = "compact",
    ...props
  },
  ref,
) {
  const variantClassName =
    variant === "card"
      ? "h-auto w-full justify-start rounded-lg px-3 py-2.5 text-left text-on-surface-variant"
      : "h-auto min-h-0 w-fit justify-start rounded-lg px-2.5 py-2 text-xs font-bold text-on-surface-variant";

  return (
    <SelectableCardButton
      className={cn(
        variantClassName,
        selected &&
          selectedStyle === "accent" &&
          "border-primary bg-primary-container text-on-primary",
        className,
      )}
      onClick={onClick}
      ref={ref}
      selected={selected}
      size="sm"
      tone="outline"
      type={type}
      {...props}
    >
      {variant === "card" ? (
        <span className="grid w-full gap-1.5 text-left">{children}</span>
      ) : (
        children
      )}
    </SelectableCardButton>
  );
});
