import type { ReactNode } from "react";

import { ButtonLink } from "@you-agent-factory/components/primitives";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import {
  CurrentSelectionDetailCode,
  CurrentSelectionDetailItem,
} from "../../../base/components/detail/current-selection-detail-item";
import { CurrentSelectionDetailSection } from "../../../base/components/detail/current-selection-detail-section";

export function DispatchDetailSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <CurrentSelectionDetailSection title={title}>
      {children}
    </CurrentSelectionDetailSection>
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
    <CurrentSelectionDetailItem
      label={label}
      value={
        href ? (
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
        )
      }
    />
  );
}
