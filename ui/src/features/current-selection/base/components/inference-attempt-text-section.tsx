import { DASHBOARD_SUPPORTING_LABEL_CLASS } from "../../../../components/ui/dashboard-typography";
import { AuthoredBodyText } from "../../../../lib/authored-body-text";
import { INFERENCE_ATTEMPT_TEXT_CLASS } from "./detail-card-shared";
import type { InferenceAttemptTextSectionProps } from "./detail-card-types";

export function InferenceAttemptTextSection({
  label,
  value,
}: InferenceAttemptTextSectionProps) {
  return (
    <div className="grid gap-1">
      <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{label}</span>
      <AuthoredBodyText
        className={INFERENCE_ATTEMPT_TEXT_CLASS}
        value={value}
      />
    </div>
  );
}
