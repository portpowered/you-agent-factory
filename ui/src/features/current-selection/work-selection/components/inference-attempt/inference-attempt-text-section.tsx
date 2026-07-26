import { Label } from "@you-agent-factory/components/primitives";
import { AuthoredBodyText } from "../../../../../lib/authored-body-text";
import type { InferenceAttemptTextSectionProps } from "../../../base/components/detail-card/detail-card-types";

export function InferenceAttemptTextSection({
  label,
  value,
}: InferenceAttemptTextSectionProps) {
  return (
    <div className="grid gap-1">
      <Label>{label}</Label>
      <AuthoredBodyText className="min-h-80 md:min-h-96" value={value} />
    </div>
  );
}
