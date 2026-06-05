import { useId, type HTMLAttributes, type ReactNode } from "react";

import { DashboardHeading } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";

export interface CurrentSelectionDetailSectionProps
  extends Omit<HTMLAttributes<HTMLElement>, "title"> {
  ariaLabel?: string;
  children: ReactNode;
  headingId?: string;
  title?: ReactNode;
}

export function CurrentSelectionDetailSection({
  ariaLabel,
  children,
  className,
  headingId,
  title,
  ...props
}: CurrentSelectionDetailSectionProps) {
  const generatedHeadingId = useId();
  const resolvedHeadingId = title ? (headingId ?? generatedHeadingId) : undefined;

  return (
    <section
      aria-label={title ? undefined : ariaLabel}
      aria-labelledby={resolvedHeadingId}
      className={cn("mt-4 grid gap-3 border-t border-outline pt-4", className)}
      {...props}
    >
      {title ? (
        <DashboardHeading as="h4" className="m-0" id={resolvedHeadingId}>
          {title}
        </DashboardHeading>
      ) : null}
      {children}
    </section>
  );
}
