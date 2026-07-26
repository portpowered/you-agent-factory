import type { ReactNode } from "react";

import { Text } from "@you-agent-factory/components/primitives";

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
