import type { ReactNode } from "react";

import { cn } from "../../../../lib/cn";
import { StandardExpandableSection } from "../../../standard-card-components/public";
import { CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS } from "./detail-card-shared";

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
      contentClassName={CURRENT_SELECTION_EXPANDABLE_SECTION_BODY_CLASS}
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
