import type { WorkStateDetailMessages } from "./work-state-detail-types";

const WORK_STATE_DETAIL_MESSAGES: Record<string, WorkStateDetailMessages> = {
  en: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `Delete ${workTypeName} ${stateName} work state`,
    topologyDeleteBlockedPrefix: "Work state cannot be deleted:",
    topologyDeleteHeading: "Factory graph",
  },
  ja: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `ワーク状態 ${workTypeName} ${stateName} を削除`,
    topologyDeleteBlockedPrefix: "ワーク状態を削除できません:",
    topologyDeleteHeading: "ファクトリグラフ",
  },
  ko: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `작업 상태 ${workTypeName} ${stateName} 삭제`,
    topologyDeleteBlockedPrefix: "작업 상태를 삭제할 수 없습니다:",
    topologyDeleteHeading: "팩토리 그래프",
  },
  zh: {
    topologyDeleteAction: (workTypeName, stateName) =>
      `删除工作状态 ${workTypeName} ${stateName}`,
    topologyDeleteBlockedPrefix: "无法删除工作状态:",
    topologyDeleteHeading: "工厂图",
  },
};

export function getWorkStateDetailMessages(
  locale?: string | null,
): WorkStateDetailMessages {
  if (!locale) {
    return WORK_STATE_DETAIL_MESSAGES.en;
  }

  const normalizedLocale = locale.toLowerCase();
  if (normalizedLocale.startsWith("ja")) {
    return WORK_STATE_DETAIL_MESSAGES.ja;
  }
  if (normalizedLocale.startsWith("ko")) {
    return WORK_STATE_DETAIL_MESSAGES.ko;
  }
  if (normalizedLocale.startsWith("zh")) {
    return WORK_STATE_DETAIL_MESSAGES.zh;
  }

  return WORK_STATE_DETAIL_MESSAGES.en;
}
