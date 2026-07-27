import { type HTMLAttributes, type ReactNode, useId } from "react";

import { Heading } from "@you-agent-factory/components/primitives";
import { cn } from "../../../../../lib/cn";

export interface CurrentSelectionContentSectionProps
  extends Omit<HTMLAttributes<HTMLElement>, "title"> {
  ariaLabel?: string;
  children: ReactNode;
  headingId?: string;
  title: ReactNode;
}

export function CurrentSelectionContentSection({
  ariaLabel,
  children,
  className,
  headingId,
  title,
  ...props
}: CurrentSelectionContentSectionProps) {
  const generatedHeadingId = useId();
  const resolvedHeadingId = headingId ?? generatedHeadingId;

  return (
    <section
      aria-label={ariaLabel}
      aria-labelledby={ariaLabel ? undefined : resolvedHeadingId}
      className={cn("mt-4 grid gap-2.5 [&_h4]:m-0", className)}
      {...props}
    >
      <Heading as="h4" className="m-0" id={resolvedHeadingId}>
        {title}
      </Heading>
      {children}
    </section>
  );
}
