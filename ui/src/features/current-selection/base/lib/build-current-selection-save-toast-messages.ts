import type { CurrentSelectionSaveEntityKind } from "../../../notifications/lib/save-notification-delivery-policy";
import { getResourceDetailMessages } from "../../resource-selection/messages/resource-detail";
import { getWorkStateDetailMessages } from "../../work-state-selection/messages/work-state-detail";
import { getWorkTypeDetailMessages } from "../../work-type-selection/messages/work-type-detail";
import { getWorkerDetailMessages } from "../../worker-selection/messages/worker-detail";
import { getWorkstationDetailMessages } from "../../workstation-selection/messages/workstation-detail";
import {
  getCurrentSelectionSaveToastCatalogMessages,
  resolveCurrentSelectionSaveToastTitles,
} from "../messages/current-selection-save-toast";
import type { CurrentSelectionSaveToastMessages } from "./current-selection-save-notifications";

export function buildCurrentSelectionSaveToastMessages({
  entityDisplayName,
  entityKind,
  locale,
}: {
  entityDisplayName: string;
  entityKind: CurrentSelectionSaveEntityKind;
  locale?: string | null;
}): CurrentSelectionSaveToastMessages {
  const toastCatalog = getCurrentSelectionSaveToastCatalogMessages(locale);
  const { saveFailedTitle, saveSuccessTitle } =
    resolveCurrentSelectionSaveToastTitles(toastCatalog, entityKind);

  switch (entityKind) {
    case "workstation": {
      const detailMessages = getWorkstationDetailMessages(locale);
      return {
        saveFailedAffectedSummary: toastCatalog.saveFailedAffectedSummary,
        saveFailedTitle,
        saveSuccessDescription: detailMessages.editableConfigurationSaveSuccess,
        saveSuccessTitle,
        staleVersionDetail:
          detailMessages.editableConfigurationSaveStaleVersionDetail,
      };
    }
    case "worker": {
      const detailMessages = getWorkerDetailMessages(locale);
      return {
        saveFailedAffectedSummary: toastCatalog.saveFailedAffectedSummary,
        saveFailedTitle,
        saveSuccessDescription:
          detailMessages.editableConfigurationSaveSuccess(entityDisplayName),
        saveSuccessTitle,
        staleVersionDetail:
          detailMessages.editableConfigurationSaveStaleVersionDetail,
      };
    }
    case "resource": {
      const detailMessages = getResourceDetailMessages(locale);
      return {
        saveFailedAffectedSummary: toastCatalog.saveFailedAffectedSummary,
        saveFailedTitle,
        saveSuccessDescription:
          detailMessages.editableConfigurationSaveSuccess(entityDisplayName),
        saveSuccessTitle,
        staleVersionDetail:
          detailMessages.editableConfigurationSaveStaleVersionDetail,
      };
    }
    case "work-type": {
      const detailMessages = getWorkTypeDetailMessages(locale);
      return {
        saveFailedAffectedSummary: toastCatalog.saveFailedAffectedSummary,
        saveFailedTitle,
        saveSuccessDescription:
          detailMessages.editableConfigurationSaveSuccess(entityDisplayName),
        saveSuccessTitle,
        staleVersionDetail:
          detailMessages.editableConfigurationSaveStaleVersionDetail,
      };
    }
    case "work-state": {
      const detailMessages = getWorkStateDetailMessages(locale);
      return {
        saveFailedAffectedSummary: toastCatalog.saveFailedAffectedSummary,
        saveFailedTitle,
        saveSuccessDescription:
          detailMessages.editableConfigurationSaveSuccess(entityDisplayName),
        saveSuccessTitle,
        staleVersionDetail:
          detailMessages.editableConfigurationSaveStaleVersionDetail,
      };
    }
  }
}
