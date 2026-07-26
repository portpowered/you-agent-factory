import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { MetadataSectionProps } from "../../../base/components/detail-card/detail-card-types";
import { CurrentSelectionDescriptionList } from "../../../base/components/detail/current-selection-description-list";
import { CurrentSelectionDetailItem } from "../../../base/components/detail/current-selection-detail-item";
import { CurrentSelectionExpandableSection } from "../../../base/components/detail/current-selection-expandable-section";
import { useCurrentSelectionDetailMessages } from "../../../base/components/presentation/current-selection-locale";

export function RequestMetadataSection({
  emptyMessage,
  metadata,
  title,
}: MetadataSectionProps) {
  const messages = useCurrentSelectionDetailMessages();
  const entries = Object.entries(metadata ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );

  return (
    <CurrentSelectionExpandableSection
      defaultExpanded
      title={title}
      toggleLabel={(expanded) =>
        expanded ? messages.collapseAction : messages.expandAction
      }
    >
      {entries.length > 0 ? (
        <CurrentSelectionDescriptionList>
          {entries.map(([key, value]) => (
            <CurrentSelectionDetailItem
              code
              key={key}
              label={key}
              value={value}
            />
          ))}
        </CurrentSelectionDescriptionList>
      ) : (
        <WidgetDetailCopy>{emptyMessage}</WidgetDetailCopy>
      )}
    </CurrentSelectionExpandableSection>
  );
}
