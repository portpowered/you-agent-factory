import {
  type HTMLAttributes,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from "react";
import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import { cn } from "../../../lib/cn";

type DataAttributes = Record<
  `data-${string}`,
  boolean | number | string | undefined
>;

export interface StandardExpandableSectionProps {
  children: ReactNode;
  className?: string;
  contentID?: string;
  contentClassName?: string;
  defaultExpanded?: boolean;
  heading: string;
  headingGroupAttributes?: HTMLAttributes<HTMLDivElement> & DataAttributes;
  headingID?: string;
  headingLevel?: 4 | 5;
  leadingVisual?: ReactNode;
  preview?: ReactNode;
  resetKey?: string;
  sectionAttributes?: HTMLAttributes<HTMLElement> & DataAttributes;
  supportingText?: ReactNode;
  toggleLabel: (context: { expanded: boolean; section: string }) => string;
}

export function StandardExpandableSection({
  children,
  className,
  contentID,
  contentClassName,
  defaultExpanded = false,
  heading,
  headingGroupAttributes,
  headingID,
  headingLevel = 5,
  leadingVisual,
  preview,
  resetKey,
  sectionAttributes,
  supportingText,
  toggleLabel,
}: StandardExpandableSectionProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const generatedContentID = useId();
  const resolvedContentID = contentID ?? generatedContentID;
  const disclosureLabel = toggleLabel({ expanded, section: heading });
  const hasContent = expanded || preview !== undefined;
  const HeadingTag = headingLevel === 4 ? "h4" : "h5";

  useEffect(() => {
    void resetKey;
    setExpanded(defaultExpanded);
  }, [defaultExpanded, resetKey]);

  return (
    <section
      {...sectionAttributes}
      aria-labelledby={headingID}
      className={cn(
        "grid gap-3 py-1.5",
        sectionAttributes?.className,
        className,
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="grid min-w-0 gap-1">
          <div
            {...headingGroupAttributes}
            className={cn(
              "flex min-w-0 items-center gap-2",
              headingGroupAttributes?.className,
            )}
          >
            {leadingVisual}
            <HeadingTag
              className={DASHBOARD_SECTION_HEADING_CLASS}
              id={headingID}
            >
              {heading}
            </HeadingTag>
          </div>
          {supportingText ? (
            <p
              className={cn(
                "m-0 text-on-surface-subtle",
                DASHBOARD_SUPPORTING_TEXT_CLASS,
              )}
            >
              {supportingText}
            </p>
          ) : null}
        </div>
        <ExpandablePanelTrigger
          aria-label={disclosureLabel}
          className="mt-0.5 h-10 min-h-0 w-10 rounded-lg"
          controlsID={resolvedContentID}
          expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          variant="outline"
        />
      </div>
      {hasContent ? (
        <div
          className={cn("grid gap-5", contentClassName)}
          id={resolvedContentID}
        >
          {expanded ? children : preview}
        </div>
      ) : null}
    </section>
  );
}
