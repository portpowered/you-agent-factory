import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { CurrentSelectionSaveEntityKind } from "../../../notifications/lib/save-notification-delivery-policy";

export interface CurrentSelectionSaveToastCatalogMessages {
  resourceSaveFailedTitle: string;
  resourceSaveSuccessTitle: string;
  saveFailedAffectedSummary: (labels: string) => string;
  workStateSaveFailedTitle: string;
  workStateSaveSuccessTitle: string;
  workTypeSaveFailedTitle: string;
  workTypeSaveSuccessTitle: string;
  workerSaveFailedTitle: string;
  workerSaveSuccessTitle: string;
  workstationSaveFailedTitle: string;
  workstationSaveSuccessTitle: string;
}

const currentSelectionSaveToastMessagesByLocale = {
  en: {
    resourceSaveFailedTitle: "Resource save failed",
    resourceSaveSuccessTitle: "Resource saved",
    saveFailedAffectedSummary: (labels: string) => `Affected: ${labels}`,
    workStateSaveFailedTitle: "Work state save failed",
    workStateSaveSuccessTitle: "Work state saved",
    workTypeSaveFailedTitle: "Work type save failed",
    workTypeSaveSuccessTitle: "Work type saved",
    workerSaveFailedTitle: "Worker save failed",
    workerSaveSuccessTitle: "Worker saved",
    workstationSaveFailedTitle: "Workstation save failed",
    workstationSaveSuccessTitle: "Workstation saved",
  },
  "zh-CN": {
    resourceSaveFailedTitle: "资源保存失败",
    resourceSaveSuccessTitle: "资源已保存",
    saveFailedAffectedSummary: (labels: string) => `受影响项：${labels}`,
    workStateSaveFailedTitle: "工作状态保存失败",
    workStateSaveSuccessTitle: "工作状态已保存",
    workTypeSaveFailedTitle: "工作类型保存失败",
    workTypeSaveSuccessTitle: "工作类型已保存",
    workerSaveFailedTitle: "工作者保存失败",
    workerSaveSuccessTitle: "工作者已保存",
    workstationSaveFailedTitle: "工位保存失败",
    workstationSaveSuccessTitle: "工位已保存",
  },
  ko: {
    resourceSaveFailedTitle: "리소스 저장 실패",
    resourceSaveSuccessTitle: "리소스 저장됨",
    saveFailedAffectedSummary: (labels: string) => `영향 대상: ${labels}`,
    workStateSaveFailedTitle: "작업 상태 저장 실패",
    workStateSaveSuccessTitle: "작업 상태 저장됨",
    workTypeSaveFailedTitle: "작업 유형 저장 실패",
    workTypeSaveSuccessTitle: "작업 유형 저장됨",
    workerSaveFailedTitle: "워커 저장 실패",
    workerSaveSuccessTitle: "워커 저장됨",
    workstationSaveFailedTitle: "워크스테이션 저장 실패",
    workstationSaveSuccessTitle: "워크스테이션 저장됨",
  },
  ja: {
    resourceSaveFailedTitle: "リソースの保存に失敗しました",
    resourceSaveSuccessTitle: "リソースを保存しました",
    saveFailedAffectedSummary: (labels: string) => `影響対象: ${labels}`,
    workStateSaveFailedTitle: "作業状態の保存に失敗しました",
    workStateSaveSuccessTitle: "作業状態を保存しました",
    workTypeSaveFailedTitle: "作業タイプの保存に失敗しました",
    workTypeSaveSuccessTitle: "作業タイプを保存しました",
    workerSaveFailedTitle: "ワーカーの保存に失敗しました",
    workerSaveSuccessTitle: "ワーカーを保存しました",
    workstationSaveFailedTitle: "ワークステーションの保存に失敗しました",
    workstationSaveSuccessTitle: "ワークステーションを保存しました",
  },
} satisfies LocalizedMessages<CurrentSelectionSaveToastCatalogMessages>;

export function getCurrentSelectionSaveToastCatalogMessages(
  locale?: string | null,
): CurrentSelectionSaveToastCatalogMessages {
  return resolveLocalizedMessages(
    currentSelectionSaveToastMessagesByLocale,
    locale,
  );
}

export function resolveCurrentSelectionSaveToastTitles(
  catalog: CurrentSelectionSaveToastCatalogMessages,
  entityKind: CurrentSelectionSaveEntityKind,
): {
  saveFailedTitle: string;
  saveSuccessTitle: string;
} {
  switch (entityKind) {
    case "workstation":
      return {
        saveFailedTitle: catalog.workstationSaveFailedTitle,
        saveSuccessTitle: catalog.workstationSaveSuccessTitle,
      };
    case "worker":
      return {
        saveFailedTitle: catalog.workerSaveFailedTitle,
        saveSuccessTitle: catalog.workerSaveSuccessTitle,
      };
    case "resource":
      return {
        saveFailedTitle: catalog.resourceSaveFailedTitle,
        saveSuccessTitle: catalog.resourceSaveSuccessTitle,
      };
    case "work-type":
      return {
        saveFailedTitle: catalog.workTypeSaveFailedTitle,
        saveSuccessTitle: catalog.workTypeSaveSuccessTitle,
      };
    case "work-state":
      return {
        saveFailedTitle: catalog.workStateSaveFailedTitle,
        saveSuccessTitle: catalog.workStateSaveSuccessTitle,
      };
  }
}
