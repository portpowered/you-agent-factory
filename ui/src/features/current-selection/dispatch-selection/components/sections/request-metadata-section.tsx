import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import type { MetadataSectionProps } from "../../../base/components/detail-card/detail-card-types";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailItem,
  CurrentSelectionExpandableSection,
  useCurrentSelectionDetailMessages,
} from "../../../base/public";

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
