import type { ReactNode } from "react";

import { DashboardText } from "../../../components/ui";

export function FactoryGraphEditorMenuItemCopy({
  description,
  label,
}: {
  description?: ReactNode;
  label: ReactNode;
}) {
  return (
    <span className="grid justify-items-start gap-0.5">
      <DashboardText
        as="span"
        className="font-semibold text-on-surface"
        variant="body"
      >
        {label}
      </DashboardText>
      {description ? (
        <DashboardText as="span" variant="supporting">
          {description}
        </DashboardText>
      ) : null}
    </span>
  );
}
