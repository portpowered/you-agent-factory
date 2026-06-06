import type { ReactElement, ReactNode } from "react";

import { cn } from "../../../../lib/cn";
import type {
  CurrentSelectionExpandableSection,
  CurrentSelectionExpandableSectionProps,
} from "./current-selection-expandable-section";

export type CurrentSelectionBodySectionElement = ReactElement<
  CurrentSelectionExpandableSectionProps,
  typeof CurrentSelectionExpandableSection
>;

export type CurrentSelectionBodySectionChild =
  | CurrentSelectionBodySectionElement
  | false
  | null
  | undefined;

export interface CurrentSelectionBodyLayoutProps {
  children:
    | CurrentSelectionBodySectionChild
    | readonly CurrentSelectionBodySectionChild[];
  className?: string;
  title: ReactNode;
}

export function CurrentSelectionBodyLayout({
  children,
  className,
  title,
}: CurrentSelectionBodyLayoutProps) {
  return (
    <div className={cn("grid", className)}>
      <p className="type-display-large">{title}</p>
      {children}
    </div>
  );
}
