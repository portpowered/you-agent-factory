import { useEffect, useRef } from "react";
import { toast } from "sonner";

import {
  buildSaveErrorStableIdentity,
  buildSaveNotificationDeliveryKey,
  GLOBAL_TOAST_DURATION_MS,
  type SaveNotificationDeliveryKey,
  shouldDeliverSaveNotification,
} from "../../../../notifications/lib/save-notification-delivery-policy";
import type { FactoryDocumentSaveState } from "../../hooks/factory-document-save-types";
import {
  CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY,
  resolveCurrentSelectionGraphDraftConflictNotification,
} from "../../lib/current-selection-graph-draft-conflict-notifications";

function buildGraphDraftConflictWarningStableIdentity() {
  return buildSaveErrorStableIdentity({
    code: "warning",
    message: CURRENT_SELECTION_GRAPH_DRAFT_CONFLICT_WARNING_KEY,
  });
}

export type CurrentSelectionGraphDraftConflictNotificationsProps = {
  documentSave: FactoryDocumentSaveState;
  graphDraftHasPendingChanges: boolean;
  isTopologyAffectingSave: boolean;
  locale?: string | null;
  saveAttemptRevision: number;
};

export function CurrentSelectionGraphDraftConflictNotifications({
  documentSave,
  graphDraftHasPendingChanges,
  isTopologyAffectingSave,
  locale,
  saveAttemptRevision,
}: CurrentSelectionGraphDraftConflictNotificationsProps) {
  const lastDeliveredDeliveryKeyRef =
    useRef<SaveNotificationDeliveryKey | null>(null);
  const notification = resolveCurrentSelectionGraphDraftConflictNotification({
    graphDraftHasPendingChanges,
    isTopologyAffectingSave,
    locale,
    saveSucceeded: documentSave.status === "success",
  });

  useEffect(() => {
    if (notification === null) {
      return;
    }

    const deliveryKey = buildSaveNotificationDeliveryKey(
      buildGraphDraftConflictWarningStableIdentity(),
      saveAttemptRevision,
    );

    if (
      !shouldDeliverSaveNotification(
        deliveryKey,
        lastDeliveredDeliveryKeyRef.current,
      )
    ) {
      return;
    }

    lastDeliveredDeliveryKeyRef.current = deliveryKey;

    toast.warning(notification.title, {
      description: notification.description,
      duration: GLOBAL_TOAST_DURATION_MS,
    });
  }, [notification, saveAttemptRevision]);

  return null;
}
