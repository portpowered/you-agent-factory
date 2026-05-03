import type { FactoryWorkItem, FactoryWorkstation } from "../../../../api/events";

export const SYSTEM_TIME_WORK_TYPE_ID = "__system_time";
const SYSTEM_TIME_PENDING_STATE = "pending";
export const SYSTEM_TIME_PENDING_PLACE_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:${SYSTEM_TIME_PENDING_STATE}`;
export const SYSTEM_TIME_EXPIRY_TRANSITION_ID = `${SYSTEM_TIME_WORK_TYPE_ID}:expire`;
const DASHBOARD_TIME_WORK_TYPE_ID = "time";
const DASHBOARD_TIME_PENDING_PLACE_ID = `${DASHBOARD_TIME_WORK_TYPE_ID}:${SYSTEM_TIME_PENDING_STATE}`;
const DASHBOARD_TIME_EXPIRY_TRANSITION_ID = `${DASHBOARD_TIME_WORK_TYPE_ID}:expire`;

export function isSystemTimeWorkType(workTypeID: string | undefined): boolean {
  return workTypeID === SYSTEM_TIME_WORK_TYPE_ID;
}

export function isSystemTimePlace(placeIDValue: string | undefined): boolean {
  return placeIDValue === SYSTEM_TIME_PENDING_PLACE_ID;
}

export function isSystemTimeWorkstation(workstation: FactoryWorkstation): boolean {
  return (workstation.id ?? workstation.name) === SYSTEM_TIME_EXPIRY_TRANSITION_ID;
}

export function isSystemTimeWorkItem(item: FactoryWorkItem): boolean {
  return isSystemTimeWorkType(item.work_type_id);
}

export function dashboardTransitionID(transitionID: string): string {
  return transitionID === SYSTEM_TIME_EXPIRY_TRANSITION_ID
    ? DASHBOARD_TIME_EXPIRY_TRANSITION_ID
    : transitionID;
}

export function dashboardWorkstationName(
  transitionID: string,
  name: string | undefined,
): string | undefined {
  if (
    transitionID === SYSTEM_TIME_EXPIRY_TRANSITION_ID &&
    (name === undefined || name === "" || name === SYSTEM_TIME_EXPIRY_TRANSITION_ID)
  ) {
    return DASHBOARD_TIME_EXPIRY_TRANSITION_ID;
  }

  return name;
}

export function dashboardPlaceID(placeIDValue: string): string {
  return isSystemTimePlace(placeIDValue) ? DASHBOARD_TIME_PENDING_PLACE_ID : placeIDValue;
}

export function dashboardWorkTypeID(workTypeID: string): string {
  return isSystemTimeWorkType(workTypeID) ? DASHBOARD_TIME_WORK_TYPE_ID : workTypeID;
}


