import {
  type LocalizedMessageCatalog,
  localizeEnumLabel,
  resolveLocalizedMessages,
} from "../../../../i18n";

export interface WorkerDetailEnumMessages {
  localizeExecutorProvider: (value: string) => string;
  localizeModelLocality: (value: string) => string;
  localizeModelProvider: (value: string) => string;
  localizeTimeoutUnit: (value: string) => string;
  localizeWorkerType: (value: string) => string;
}

const workerDetailEnumMessagesByLocale = {
  en: {
    localizeModelLocality: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLOUD: "Cloud",
          LOCAL: "Local",
        },
        locale: "en",
        value,
      }),
    localizeExecutorProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          SCRIPT_WRAP: "Script wrap",
        },
        locale: "en",
        value,
      }),
    localizeModelProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLAUDE: "Claude",
          CODEX: "Codex",
          ANTIGRAVITY: "Antigravity",
        },
        locale: "en",
        value,
      }),
    localizeTimeoutUnit: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          h: "Hours",
          m: "Minutes",
          s: "Seconds",
        },
        locale: "en",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          AGENT_WORKER: "Agent worker",
          HOSTED_WORKER: "Hosted worker (legacy)",
          INFERENCE_WORKER: "Inference worker",
          MODEL_WORKER: "Model worker (legacy)",
          POLLER_WORKER: "Poller worker",
          SCRIPT_WORKER: "Script worker",
        },
        locale: "en",
        value,
      }),
  },
  ja: {
    localizeModelLocality: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLOUD: "クラウド",
          LOCAL: "ローカル",
        },
        locale: "ja",
        value,
      }),
    localizeExecutorProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          SCRIPT_WRAP: "スクリプトラップ",
        },
        locale: "ja",
        value,
      }),
    localizeModelProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLAUDE: "Claude",
          CODEX: "Codex",
          ANTIGRAVITY: "Antigravity",
        },
        locale: "ja",
        value,
      }),
    localizeTimeoutUnit: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          h: "時間",
          m: "分",
          s: "秒",
        },
        locale: "ja",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          AGENT_WORKER: "エージェントワーカー",
          HOSTED_WORKER: "ホスト型ワーカー（レガシー）",
          INFERENCE_WORKER: "推論ワーカー",
          MODEL_WORKER: "モデルワーカー（レガシー）",
          POLLER_WORKER: "ポーラーワーカー",
          SCRIPT_WORKER: "スクリプトワーカー",
        },
        locale: "ja",
        value,
      }),
  },
  ko: {
    localizeModelLocality: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLOUD: "클라우드",
          LOCAL: "로컬",
        },
        locale: "ko",
        value,
      }),
    localizeExecutorProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          SCRIPT_WRAP: "스크립트 래핑",
        },
        locale: "ko",
        value,
      }),
    localizeModelProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLAUDE: "Claude",
          CODEX: "Codex",
          ANTIGRAVITY: "Antigravity",
        },
        locale: "ko",
        value,
      }),
    localizeTimeoutUnit: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          h: "시간",
          m: "분",
          s: "초",
        },
        locale: "ko",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          AGENT_WORKER: "에이전트 워커",
          HOSTED_WORKER: "호스티드 워커(레거시)",
          INFERENCE_WORKER: "추론 워커",
          MODEL_WORKER: "모델 워커(레거시)",
          POLLER_WORKER: "폴러 워커",
          SCRIPT_WORKER: "스크립트 워커",
        },
        locale: "ko",
        value,
      }),
  },
  "zh-CN": {
    localizeModelLocality: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLOUD: "云端",
          LOCAL: "本地",
        },
        locale: "zh-CN",
        value,
      }),
    localizeExecutorProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          SCRIPT_WRAP: "脚本包装",
        },
        locale: "zh-CN",
        value,
      }),
    localizeModelProvider: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          CLAUDE: "Claude",
          CODEX: "Codex",
          ANTIGRAVITY: "Antigravity",
        },
        locale: "zh-CN",
        value,
      }),
    localizeTimeoutUnit: (value: string) =>
      localizeEnumLabel({
        category: "type",
        labels: {
          h: "小时",
          m: "分钟",
          s: "秒",
        },
        locale: "zh-CN",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          AGENT_WORKER: "Agent worker",
          HOSTED_WORKER: "托管 worker（旧版）",
          INFERENCE_WORKER: "推理 worker",
          MODEL_WORKER: "模型 worker（旧版）",
          POLLER_WORKER: "轮询 worker",
          SCRIPT_WORKER: "脚本 worker",
        },
        locale: "zh-CN",
        value,
      }),
  },
} satisfies LocalizedMessageCatalog<WorkerDetailEnumMessages>;

export function getWorkerDetailEnumMessages(
  locale?: string | null,
): WorkerDetailEnumMessages {
  return resolveLocalizedMessages(workerDetailEnumMessagesByLocale, locale);
}
