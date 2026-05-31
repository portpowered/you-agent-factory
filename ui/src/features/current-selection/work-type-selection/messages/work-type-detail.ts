import type { WorkTypeDetailMessages } from "./work-type-detail-types";

const WORK_TYPE_DETAIL_MESSAGES: Record<string, WorkTypeDetailMessages> = {
  en: {
    topologyDeleteAction: (workTypeName) => `Delete ${workTypeName} work type`,
    topologyDeleteBlockedPrefix: "Work type cannot be deleted:",
    topologyDeleteHeading: "Factory graph",
  },
  ja: {
    topologyDeleteAction: (workTypeName) => `ワークタイプ ${workTypeName} を削除`,
    topologyDeleteBlockedPrefix: "ワークタイプを削除できません:",
    topologyDeleteHeading: "ファクトリグラフ",
  },
  ko: {
    topologyDeleteAction: (workTypeName) => `작업 유형 ${workTypeName} 삭제`,
    topologyDeleteBlockedPrefix: "작업 유형을 삭제할 수 없습니다:",
    topologyDeleteHeading: "팩토리 그래프",
  },
  zh: {
    topologyDeleteAction: (workTypeName) => `删除工作类型 ${workTypeName}`,
    topologyDeleteBlockedPrefix: "无法删除工作类型:",
    topologyDeleteHeading: "工厂图",
  },
};

export function getWorkTypeDetailMessages(
  locale?: string | null,
): WorkTypeDetailMessages {
  if (!locale) {
    return WORK_TYPE_DETAIL_MESSAGES.en;
  }

  const normalizedLocale = locale.toLowerCase();
  if (normalizedLocale.startsWith("ja")) {
    return WORK_TYPE_DETAIL_MESSAGES.ja;
  }
  if (normalizedLocale.startsWith("ko")) {
    return WORK_TYPE_DETAIL_MESSAGES.ko;
  }
  if (normalizedLocale.startsWith("zh")) {
    return WORK_TYPE_DETAIL_MESSAGES.zh;
  }

  return WORK_TYPE_DETAIL_MESSAGES.en;
}
