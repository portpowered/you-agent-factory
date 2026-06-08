import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "./notification-toast-duration";

export type SaveNotificationKind = "error" | "success" | "info";

export type SaveNotificationStableIdentity = {
  kind: SaveNotificationKind;
  stableId: string;
};

export type SaveNotificationDeliveryKey = string;

function normalizeSaveNotificationMessage(message: string): string {
  return message.trim();
}

/**
 * Stable identity for save errors: kind plus normalized message and optional code.
 * Cross-attempt retries with the same message share identity; delivery keys still
 * differ when save-attempt revision increments.
 */
export function buildSaveErrorStableIdentity(input: {
  message: string;
  code?: string | null;
}): SaveNotificationStableIdentity {
  const normalizedMessage = normalizeSaveNotificationMessage(input.message);
  const normalizedCode =
    typeof input.code === "string" && input.code.trim().length > 0
      ? input.code.trim()
      : null;

  const stableId =
    normalizedCode === null
      ? normalizedMessage
      : `${normalizedCode}:${normalizedMessage}`;

  return {
    kind: "error",
    stableId,
  };
}

/** Stable identity for a graph save success toast (one per save surface). */
export function buildSaveSuccessStableIdentity(): SaveNotificationStableIdentity {
  return {
    kind: "success",
    stableId: "graph-save-success",
  };
}

export type CurrentSelectionSaveEntityKind =
  | "doc"
  | "workstation"
  | "worker"
  | "resource"
  | "work-type"
  | "work-state";

/** Stable identity for a current-selection entity save success toast. */
export function buildCurrentSelectionSaveSuccessStableIdentity(
  entityKind: CurrentSelectionSaveEntityKind,
): SaveNotificationStableIdentity {
  return {
    kind: "success",
    stableId: `${entityKind}-save-success`,
  };
}

/**
 * Attempt-scoped delivery key. Includes save-attempt revision so operator retries
 * always qualify as new delivery even when stable identity is unchanged.
 */
export function buildSaveNotificationDeliveryKey(
  identity: SaveNotificationStableIdentity,
  saveAttemptRevision: number,
): SaveNotificationDeliveryKey {
  return `${identity.kind}:${identity.stableId}#${saveAttemptRevision}`;
}

/**
 * Burst-only dedupe: suppress when the same delivery key was already shown for
 * this attempt outcome (e.g. React strict-mode or prop rerenders). Does not
 * suppress across attempts because revision is part of the delivery key.
 */
export function shouldDeliverSaveNotification(
  deliveryKey: SaveNotificationDeliveryKey,
  lastDeliveredDeliveryKey: SaveNotificationDeliveryKey | null,
): boolean {
  return deliveryKey !== lastDeliveredDeliveryKey;
}

export type SaveNotificationToastOptions = {
  description: string;
  duration: number;
};

/** Sonner options for save error toasts (persistent until dismissed). */
export function buildSaveErrorToastOptions(
  description: string,
): SaveNotificationToastOptions {
  return {
    description,
    duration: PERSISTENT_TOAST_DURATION_MS,
  };
}

/** Sonner options for save success toasts (existing global TTL). */
export function buildSaveSuccessToastOptions(
  description: string,
): SaveNotificationToastOptions {
  return {
    description,
    duration: GLOBAL_TOAST_DURATION_MS,
  };
}

export {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "./notification-toast-duration";
