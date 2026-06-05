import type { ReactNode } from "react";

import { ButtonLink } from "../../../../components/ui";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailCode,
  CurrentSelectionDetailValue,
  CurrentSelectionLabel,
} from "../../base/public";

export function DispatchDetailSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <section
      aria-label={title}
      className="mt-3 grid gap-2 border-t border-outline pt-3"
    >
      <CurrentSelectionLabel>{title}</CurrentSelectionLabel>
      {children}
    </section>
  );
}

export function DispatchDetailList({
  entries,
}: {
  entries: Array<{
    code?: boolean;
    href?: string;
    label: string;
    title?: string;
    value?: string;
  }>;
}) {
  const populatedEntries = entries.filter((entry) => entry.value);
  if (populatedEntries.length === 0) {
    return null;
  }

  return (
    <CurrentSelectionDescriptionList>
      {populatedEntries.map((entry) => (
        <DispatchDetailListItem
          code={entry.code}
          href={entry.href}
          key={entry.label}
          label={entry.label}
          title={entry.title}
          value={entry.value}
        />
      ))}
    </CurrentSelectionDescriptionList>
  );
}

function DispatchDetailListItem({
  code = false,
  href,
  label,
  title,
  value,
}: {
  code?: boolean;
  href?: string;
  label: string;
  title?: string;
  value?: string;
}) {
  if (!value) {
    return null;
  }

  return (
    <div>
      <dt>{label}</dt>
      <CurrentSelectionDetailValue>
        {href ? (
          <ButtonLink
            className="w-fit"
            href={href}
            size="sm"
            title={title}
            tone="outline"
          >
            {value}
          </ButtonLink>
        ) : code ? (
          <CurrentSelectionDetailCode>{value}</CurrentSelectionDetailCode>
        ) : (
          value
        )}
      </CurrentSelectionDetailValue>
    </div>
  );
}
