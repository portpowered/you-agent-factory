import type { ReactNode } from "react";

import {
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import { cn } from "../../../../lib/cn";
import { HISTORY_HEADER_CLASS } from "./detail-card-shared";

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
    <div className={HISTORY_HEADER_CLASS}>
      <div className="grid min-w-0 gap-1">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS} id={headingId}>
          {title}
        </h4>
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
      {action}
    </div>
  );
}
