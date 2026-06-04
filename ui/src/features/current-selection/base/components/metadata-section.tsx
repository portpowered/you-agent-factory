import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../../components/ui/dashboard-typography";
import { DETAIL_COPY_CLASS } from "../../../../components/ui/widget-frame";
import {
  INFERENCE_ATTEMPT_DETAIL_CLASS,
  RUNTIME_DETAIL_CODE_CLASS,
  RUNTIME_DETAIL_VALUE_CLASS,
  RUNTIME_DETAILS_SECTION_CLASS,
} from "./detail-card-shared";
import type { MetadataSectionProps } from "./detail-card-types";

export function MetadataSection({
  emptyMessage,
  metadata,
  title,
}: MetadataSectionProps) {
  const entries = Object.entries(metadata ?? {}).sort(([left], [right]) =>
    left.localeCompare(right),
  );

  return (
    <section aria-label={title} className={RUNTIME_DETAILS_SECTION_CLASS}>
      <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{title}</h4>
      {entries.length > 0 ? (
        <dl className={INFERENCE_ATTEMPT_DETAIL_CLASS}>
          {entries.map(([key, value]) => (
            <div key={key}>
              <dt>{key}</dt>
              <dd className={RUNTIME_DETAIL_VALUE_CLASS}>
                <code className={RUNTIME_DETAIL_CODE_CLASS}>{value}</code>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <p className={DETAIL_COPY_CLASS}>{emptyMessage}</p>
      )}
    </section>
  );
}
