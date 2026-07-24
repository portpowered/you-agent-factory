import type { ReactNode } from "react";

import { Text } from "../../../../components/ui";

export function FactoryGraphEditorMenuItemCopy({
  description,
  label,
}: {
  description?: ReactNode;
  label: ReactNode;
}) {
  return (
    <span className="grid justify-items-start gap-0.5">
      <Text as="span" className="font-semibold text-on-surface" variant="body">
        {label}
      </Text>
      {description ? (
        <Text as="span" variant="supporting">
          {description}
        </Text>
      ) : null}
    </span>
  );
}
