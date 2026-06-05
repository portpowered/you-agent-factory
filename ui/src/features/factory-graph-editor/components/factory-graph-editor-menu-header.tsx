import type { ReactNode } from "react";

import { DashboardText } from "../../../components/ui";

export function FactoryGraphEditorMenuHeader({
  description,
  title,
}: {
  description?: ReactNode;
  title: ReactNode;
}) {
  return (
    <div className="grid gap-1">
      <DashboardText
        as="p"
        className="m-0 font-semibold text-on-surface"
        variant="body"
      >
        {title}
      </DashboardText>
      {description ? (
        <DashboardText as="p" className="m-0" variant="supporting">
          {description}
        </DashboardText>
      ) : null}
    </div>
  );
}
