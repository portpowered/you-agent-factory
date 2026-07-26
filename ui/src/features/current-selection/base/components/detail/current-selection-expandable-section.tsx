import { type ReactNode, useId } from "react";
import { cn } from "../../../../../lib/cn";
import { StandardExpandableSection } from "../../../../standard-card-components/components/standard-expandable-section";

export interface CurrentSelectionExpandableSectionProps {
  children: ReactNode;
  className?: string;
  contentId?: string;
  preview?: ReactNode;
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
  preview,
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
      contentID={contentId}
      defaultExpanded={defaultExpanded}
      heading={title}
      headingID={resolvedHeadingId}
      headingLevel={4}
      preview={preview}
      resetKey={resetKey}
      supportingText={supportingText}
      toggleLabel={({ expanded }) => toggleLabel(expanded)}
    >
      {children}
    </StandardExpandableSection>
  );
}
