import {
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
} from "./detail-card-shared";
import type { InferenceAttemptDetailProps } from "./detail-card-types";

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
    <div>
      <dt>{label}</dt>
      <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
        {code ? (
          <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
        ) : rawValue ? (
          <span title={rawValue}>{value}</span>
        ) : (
          value
        )}
      </dd>
    </div>
  );
}
