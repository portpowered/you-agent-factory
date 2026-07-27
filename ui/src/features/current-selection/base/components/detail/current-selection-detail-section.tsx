import { type HTMLAttributes, type ReactNode, useId } from "react";

import { Heading } from "@you-agent-factory/components/primitives";
import { cn } from "../../../../../lib/cn";

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
  const resolvedHeadingId = title
    ? (headingId ?? generatedHeadingId)
    : undefined;

  return (
    <section
      aria-label={title ? undefined : ariaLabel}
      aria-labelledby={resolvedHeadingId}
      className={cn("mt-4 grid gap-3 border-t border-outline pt-4", className)}
      {...props}
    >
      {title ? (
        <Heading as="h4" className="m-0" id={resolvedHeadingId}>
          {title}
        </Heading>
      ) : null}
      {children}
    </section>
  );
}
