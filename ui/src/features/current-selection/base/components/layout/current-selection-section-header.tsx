import type { ReactNode } from "react";

import {
  DashboardHeading,
  DashboardText,
  surfacePanelVariants,
} from "../../../../../components/ui";

export function CurrentSelectionSectionHeader({
  action,
  headingId,
  supportingText,
  title,
}: {
  action?: ReactNode;
  headingId: string;
  supportingText?: ReactNode;
  title: string;
}) {
  return (
    <div
      className={surfacePanelVariants({
        className:
          "flex items-center justify-between gap-3 px-3 py-2 [&_h4]:m-0",
        radius: "lg",
      })}
    >
      <div className="grid min-w-0 gap-1">
        <DashboardHeading as="h4" id={headingId}>
          {title}
        </DashboardHeading>
        {supportingText ? (
          <DashboardText
            className="m-0 text-on-surface-subtle"
            variant="supporting"
          >
            {supportingText}
          </DashboardText>
        ) : null}
      </div>
      {action}
    </div>
  );
}
