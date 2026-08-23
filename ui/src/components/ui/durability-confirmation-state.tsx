import { ConfirmationState } from "../../api/generated/openapi";
import { DashboardStatusPill } from "./dashboard-status-pill";

export function normalizeDurabilityConfirmationState(
  state: string | null | undefined,
): ConfirmationState {
  return state === ConfirmationState.CONFIRMED
    ? ConfirmationState.CONFIRMED
    : ConfirmationState.UNCONFIRMED;
}

export function DurabilityConfirmationState({
  label,
  state,
}: {
  label: string;
  state?: string | null;
}) {
  const normalizedState = normalizeDurabilityConfirmationState(state);
  return (
    <DashboardStatusPill
      aria-label={`${label}: ${normalizedState}`}
      role="status"
      size="compact"
      tone={
        normalizedState === ConfirmationState.CONFIRMED ? "success" : "warning"
      }
    >
      {label}: {normalizedState}
    </DashboardStatusPill>
  );
}
