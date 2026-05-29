import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";
import type { WorkerDetailMessages } from "./worker-detail-types";
import { getWorkerDetailEnumMessages } from "./worker-detail-enums";

type WorkerDetailCatalogMessages = Omit<
  WorkerDetailMessages,
  "localizeExecutorProvider" | "localizeModelProvider" | "localizeWorkerType"
>;

const workerDetailMessagesByLocale = {
  en: {
    configurationEmpty:
      "This running factory definition does not include the selected worker.",
    configurationErrorPrefix: "Worker definition unavailable.",
    configurationLoading: "Loading the current factory definition for this worker.",
    executorProviderLabel: "Executor provider",
    modelLabel: "Model",
    modelProviderLabel: "Model provider",
    notConfiguredValue: "Not configured",
    referencingWorkstationsEmpty:
      "No workstations reference this worker in the running factory definition.",
    referencingWorkstationsHeading: "Referencing workstations",
    summaryHeading: "Summary",
    typeLabel: "Worker type",
    unknownTypeValue: "Unknown",
  },
  ja: {
    configurationEmpty:
      "実行中のファクトリ定義に選択したワーカーが含まれていません。",
    configurationErrorPrefix: "ワーカー定義を取得できません。",
    configurationLoading:
      "このワーカーの現在のファクトリ定義を読み込んでいます。",
    executorProviderLabel: "実行プロバイダー",
    modelLabel: "モデル",
    modelProviderLabel: "モデルプロバイダー",
    notConfiguredValue: "未設定",
    referencingWorkstationsEmpty:
      "実行中のファクトリ定義でこのワーカーを参照するワークステーションはありません。",
    referencingWorkstationsHeading: "参照ワークステーション",
    summaryHeading: "概要",
    typeLabel: "ワーカー種別",
    unknownTypeValue: "不明",
  },
  ko: {
    configurationEmpty:
      "실행 중인 팩토리 정의에 선택한 워커가 포함되어 있지 않습니다.",
    configurationErrorPrefix: "워커 정의를 사용할 수 없습니다.",
    configurationLoading:
      "이 워커의 현재 팩토리 정의를 불러오는 중입니다.",
    executorProviderLabel: "실행자 제공자",
    modelLabel: "모델",
    modelProviderLabel: "모델 제공자",
    notConfiguredValue: "구성되지 않음",
    referencingWorkstationsEmpty:
      "실행 중인 팩토리 정의에서 이 워커를 참조하는 워크스테이션이 없습니다.",
    referencingWorkstationsHeading: "참조 워크스테이션",
    summaryHeading: "요약",
    typeLabel: "워커 유형",
    unknownTypeValue: "알 수 없음",
  },
  "zh-CN": {
    configurationEmpty: "运行中的工厂定义不包含所选 worker。",
    configurationErrorPrefix: "无法加载 worker 定义。",
    configurationLoading: "正在加载此 worker 的当前工厂定义。",
    executorProviderLabel: "执行器 provider",
    modelLabel: "模型",
    modelProviderLabel: "模型 provider",
    notConfiguredValue: "未配置",
    referencingWorkstationsEmpty:
      "运行中的工厂定义中没有 workstation 引用此 worker。",
    referencingWorkstationsHeading: "引用 workstation",
    summaryHeading: "摘要",
    typeLabel: "Worker 类型",
    unknownTypeValue: "未知",
  },
} satisfies LocalizedMessages<WorkerDetailCatalogMessages>;

export { workerDetailMessagesByLocale };

export function getWorkerDetailMessages(
  locale?: string | null,
): WorkerDetailMessages {
  const enumMessages = getWorkerDetailEnumMessages(locale);
  const catalog = resolveLocalizedMessages(workerDetailMessagesByLocale, locale);

  return {
    ...catalog,
    ...enumMessages,
  };
}
