import { forwardRef, type HTMLAttributes, type ReactNode } from "react";
import { surfacePanelVariants } from "@you-agent-factory/components/layout";
import { cn } from "../../../../lib/cn";
import { CurrentSelectionExecutionPill } from "../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../base/components/presentation/current-selection-supporting-text";

export interface CurrentSelectionHistoryCardProps
  extends HTMLAttributes<HTMLElement> {
  highlighted?: boolean;
}

export const CurrentSelectionHistoryCard = forwardRef<
  HTMLElement,
  CurrentSelectionHistoryCardProps
>(function CurrentSelectionHistoryCard(
  { className, highlighted = false, ...props },
  ref,
) {
  return (
    <article
      className={surfacePanelVariants({
        className: cn("grid min-w-0 gap-2.5", className),
        radius: "lg",
        surface: "high",
        tone: highlighted ? "accent" : "default",
      })}
      ref={ref}
      {...props}
    />
  );
});

export interface CurrentSelectionHistoryCardHeaderProps
  extends Omit<HTMLAttributes<HTMLDivElement>, "title"> {
  badges?: ReactNode;
  identifier?: ReactNode;
  subtitle?: ReactNode;
  trailingContent?: ReactNode;
  title: ReactNode;
  titleClassName?: string;
}

export const CurrentSelectionHistoryCardHeader = forwardRef<
  HTMLDivElement,
  CurrentSelectionHistoryCardHeaderProps
>(function CurrentSelectionHistoryCardHeader(
  {
    badges,
    className,
    identifier,
    subtitle,
    title,
    titleClassName,
    trailingContent,
    ...props
  },
  ref,
) {
  return (
    <div
      className={cn("flex items-start justify-between gap-3", className)}
      ref={ref}
      {...props}
    >
      <div className="grid min-w-0 gap-1">
        <strong
          className={cn("min-w-0 [overflow-wrap:anywhere]", titleClassName)}
        >
          {title}
        </strong>
        {subtitle || badges ? (
          <div className="flex flex-wrap items-center gap-2">
            {subtitle ? (
              <CurrentSelectionSupportingText>
                {subtitle}
              </CurrentSelectionSupportingText>
            ) : null}
            {badges}
          </div>
        ) : null}
      </div>
      {trailingContent ? (
        <div className="shrink-0 self-start">{trailingContent}</div>
      ) : null}
      {identifier ? (
        <CurrentSelectionExecutionPill>
          {identifier}
        </CurrentSelectionExecutionPill>
      ) : null}
    </div>
  );
});
