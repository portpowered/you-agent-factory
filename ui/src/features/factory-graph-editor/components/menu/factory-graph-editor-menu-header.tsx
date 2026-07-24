import type { ReactNode } from "react";

import { Text } from "../../../../components/ui";

export function FactoryGraphEditorMenuHeader({
  description,
  title,
}: {
  description?: ReactNode;
  title: ReactNode;
}) {
  return (
    <div className="grid gap-1">
      <Text as="p" className="m-0 font-semibold text-on-surface" variant="body">
        {title}
      </Text>
      {description ? (
        <Text as="p" className="m-0" variant="supporting">
          {description}
        </Text>
      ) : null}
    </div>
  );
}
