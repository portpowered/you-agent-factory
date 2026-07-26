import {
  type HTMLAttributes,
  type ReactNode,
  useEffect,
  useId,
  useState,
} from "react";
import { Heading, Text } from "@you-agent-factory/components/primitives";
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
        "grid w-full self-start gap-3 py-1.5",
        sectionAttributes?.className,
        className,
      )}
    >
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3">
        <div className="grid min-w-0 gap-1">
          <div
            {...headingGroupAttributes}
            className={cn(
              "flex min-w-0 items-center gap-2",
              headingGroupAttributes?.className,
            )}
          >
            {leadingVisual}
            <Heading as={HeadingTag} id={headingID}>
              {heading}
            </Heading>
          </div>
          {supportingText ? (
            <Text className="m-0 text-on-surface-subtle" variant="supporting">
              {supportingText}
            </Text>
          ) : null}
        </div>
        <ExpandablePanelTrigger
          aria-label={disclosureLabel}
          className="h-10 min-h-0 w-10 self-start rounded-lg"
          controlsID={resolvedContentID}
          expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          variant="outline"
        />
      </div>
      {hasContent ? (
        <div className={cn("grid", contentClassName)} id={resolvedContentID}>
          {expanded ? children : preview}
        </div>
      ) : null}
    </section>
  );
}
