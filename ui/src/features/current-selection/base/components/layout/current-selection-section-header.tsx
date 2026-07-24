import type { ReactNode } from "react";

import {
  Heading,
  surfacePanelVariants,
  Text,
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
        <Heading as="h4" id={headingId}>
          {title}
        </Heading>
        {supportingText ? (
          <Text className="m-0 text-on-surface-subtle" variant="supporting">
            {supportingText}
          </Text>
        ) : null}
      </div>
      {action}
    </div>
  );
}
