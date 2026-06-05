import type { ReactNode } from "react";

import { surfacePanelVariants } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import { StandardExpandableSection } from "../../../standard-card-components/public";

export function CurrentSelectionExpandableSection({
  children,
  className,
  contentId,
  defaultExpanded,
  headingId,
  resetKey,
  supportingText,
  title,
  toggleLabel,
}: {
  children: ReactNode;
  className?: string;
  contentId?: string;
  defaultExpanded?: boolean;
  headingId?: string;
  resetKey?: string;
  supportingText?: ReactNode;
  title: string;
  toggleLabel: (expanded: boolean) => string;
}) {
  return (
    <StandardExpandableSection
      className={cn("mt-4 gap-2.5 py-0 [&_h4]:m-0", className)}
      contentClassName={surfacePanelVariants({
        className: "grid gap-3",
        radius: "2xl",
      })}
      contentID={contentId}
      defaultExpanded={defaultExpanded}
      heading={title}
      headingID={headingId}
      headingLevel={4}
      resetKey={resetKey}
      supportingText={supportingText}
      toggleLabel={({ expanded }) => toggleLabel(expanded)}
    >
      {children}
    </StandardExpandableSection>
  );
}
