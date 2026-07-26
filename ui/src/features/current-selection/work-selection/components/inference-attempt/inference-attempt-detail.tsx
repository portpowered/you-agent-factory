import type { InferenceAttemptDetailProps } from "../../../base/components/detail-card/detail-card-types";
import { CurrentSelectionDetailItem } from "../../../base/components/detail/current-selection-detail-item";

export function InferenceAttemptDetail({
  code = false,
  label,
  rawValue,
  value,
}: InferenceAttemptDetailProps) {
  if (value === undefined || value === "") {
    return null;
  }

  return (
    <CurrentSelectionDetailItem
      code={code}
      label={label}
      rawValue={rawValue}
      value={value}
    />
  );
}
