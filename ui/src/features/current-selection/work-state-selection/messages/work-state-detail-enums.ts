import type { EditableWorkStateDraft } from "../../../current-factory-definition/lib/work-state-editable-values";
import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";

type WorkStateType = EditableWorkStateDraft["type"];

const workStateTypeLabelsByLocale = {
  en: {
    FAILED: "Failed",
    INITIAL: "Initial",
    PROCESSING: "Processing",
    TERMINAL: "Completed",
  },
  ja: {
    FAILED: "失敗",
    INITIAL: "初期",
    PROCESSING: "処理中",
    TERMINAL: "完了",
  },
  ko: {
    FAILED: "실패",
    INITIAL: "초기",
    PROCESSING: "처리 중",
    TERMINAL: "완료",
  },
  "zh-CN": {
    FAILED: "失败",
    INITIAL: "初始",
    PROCESSING: "处理中",
    TERMINAL: "终止",
  },
} satisfies LocalizedMessages<Record<WorkStateType, string>>;

export function localizeWorkStateType(
  stateType: WorkStateType,
  locale?: string | null,
): string {
  const labels = resolveLocalizedMessages(workStateTypeLabelsByLocale, locale);
  return labels[stateType] ?? stateType;
}
