import type { DashboardPlaceRef } from "../../api/dashboard/types";

export interface DashboardPlaceLabelParts {
  rawLabel: string;
  stateValue: string;
  workType: string;
}

export function formatDashboardPlaceLabel(place: DashboardPlaceRef): string {
  if (place.type_id && place.state_value) {
    return `${place.type_id}:${place.state_value}`;
  }
  return place.place_id;
}

export function getDashboardPlaceLabelParts(
  place: DashboardPlaceRef,
): DashboardPlaceLabelParts {
  return {
    rawLabel: formatDashboardPlaceLabel(place),
    stateValue: place.state_value ?? place.place_id,
    workType: place.type_id ?? "work",
  };
}

