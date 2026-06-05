import { DetailCopy } from "../../../../components/ui/widget-frame";
import type { MetadataSectionProps } from "../../base/components/detail-card-types";
import {
  CurrentSelectionDescriptionList,
  CurrentSelectionDetailItem,
  CurrentSelectionDetailSection,
} from "../../base/public";

export function RequestMetadataSection({
  emptyMessage,
  metadata,
  title,
}: MetadataSectionProps) {
  const entries = Object.entries(metadata ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );

  return (
    <CurrentSelectionDetailSection title={title}>
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
        <DetailCopy>{emptyMessage}</DetailCopy>
      )}
    </CurrentSelectionDetailSection>
  );
}
