import { AlertPanel, AlertPanelText } from "@you-agent-factory/components/feedback";
import type { SubmitWorkStatus } from "./submit-work-card";

const SUBMIT_WORK_STATUS_TONE_BY_KIND: Record<
  SubmitWorkStatus["kind"],
  "danger" | "info" | "neutral" | "success"
> = {
  error: "danger",
  guidance: "neutral",
  submitting: "info",
  success: "success",
  "validation-error": "danger",
};

export interface SubmitWorkStatusPanelProps {
  id: string;
  status: SubmitWorkStatus;
}

export function SubmitWorkStatusPanel({
  id,
  status,
}: SubmitWorkStatusPanelProps) {
  const isErrorStatus =
    status.kind === "error" || status.kind === "validation-error";

  return (
    <AlertPanel
      className="min-w-0 max-w-xl leading-relaxed"
      compact
      id={id}
      role={isErrorStatus ? "alert" : "status"}
      tone={SUBMIT_WORK_STATUS_TONE_BY_KIND[status.kind]}
      variant="empty"
    >
      <AlertPanelText>{status.message}</AlertPanelText>
    </AlertPanel>
  );
}
