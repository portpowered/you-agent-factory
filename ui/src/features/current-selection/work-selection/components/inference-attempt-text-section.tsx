import { DashboardLabel } from "../../../../components/ui";
import { AuthoredBodyText } from "../../../../lib/authored-body-text";
import type { InferenceAttemptTextSectionProps } from "../../base/components/detail-card-types";

export function InferenceAttemptTextSection({
  label,
  value,
}: InferenceAttemptTextSectionProps) {
  return (
    <div className="grid gap-1">
      <DashboardLabel>{label}</DashboardLabel>
      <AuthoredBodyText
        className="min-h-[20rem] md:min-h-[26rem] lg:min-h-[min(70vh,36rem)]"
        value={value}
      />
    </div>
  );
}
