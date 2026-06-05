import type { DashboardPlaceRef } from "../../../../api/dashboard/types";
import type { CurrentSelectionDetailMessages } from "../messages/current-selection-detail";

export function isTerminalOrFailedPlace(place: DashboardPlaceRef): boolean {
  return (
    place.state_category === "TERMINAL" || place.state_category === "FAILED"
  );
}

export function emptyStatePlaceMessage(
  messages: Pick<
    CurrentSelectionDetailMessages,
    | "noCurrentWorkInPlace"
    | "noWorkRecordedAtSelectedTick"
    | "selectedTickWorkUnavailable"
  >,
  usesRetainedWorkItems: boolean,
  tokenCount: number,
): string {
  if (!usesRetainedWorkItems) {
    return messages.noCurrentWorkInPlace;
  }

  if (tokenCount > 0) {
    return messages.selectedTickWorkUnavailable;
  }

  return messages.noWorkRecordedAtSelectedTick;
}

export function normalizeDetailText(
  value: string | undefined,
): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}
