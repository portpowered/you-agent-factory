import { useId, type ReactNode } from "react";

import { surfacePanelVariants } from "../../../../components/ui";
import { cn } from "../../../../lib/cn";
import { StandardExpandableSection } from "../../../standard-card-components/public";

export interface CurrentSelectionExpandableSectionProps {
  children: ReactNode;
  className?: string;
  contentId?: string;
  defaultExpanded?: boolean;
  headingId?: string;
  resetKey?: string;
  supportingText?: ReactNode;
  title: string;
  toggleLabel: (expanded: boolean) => string;
}

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
}: CurrentSelectionExpandableSectionProps) {
  const generatedHeadingId = useId();
  const resolvedHeadingId = headingId ?? generatedHeadingId;

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
      headingID={resolvedHeadingId}
      headingLevel={4}
      resetKey={resetKey}
      supportingText={supportingText}
      toggleLabel={({ expanded }) => toggleLabel(expanded)}
    >
      {children}
    </StandardExpandableSection>
  );
}
