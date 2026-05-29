import {
  localizeEnumLabel,
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../../i18n";

export interface WorkerDetailEnumMessages {
  localizeExecutorProvider: (value: string) => string;
  localizeModelLocality: (value: string) => string;
  localizeModelProvider: (value: string) => string;
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
          CURSOR: "Cursor",
          GEMINI: "Gemini",
          KIRO: "Kiro",
          OPENCODE: "OpenCode",
        },
        locale: "en",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          HOSTED_WORKER: "Hosted worker",
          MODEL_WORKER: "Model worker",
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
          CURSOR: "Cursor",
          GEMINI: "Gemini",
          KIRO: "Kiro",
          OPENCODE: "OpenCode",
        },
        locale: "ja",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          HOSTED_WORKER: "ホスト型ワーカー",
          MODEL_WORKER: "モデルワーカー",
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
          CURSOR: "Cursor",
          GEMINI: "Gemini",
          KIRO: "Kiro",
          OPENCODE: "OpenCode",
        },
        locale: "ko",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          HOSTED_WORKER: "호스티드 워커",
          MODEL_WORKER: "모델 워커",
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
          CURSOR: "Cursor",
          GEMINI: "Gemini",
          KIRO: "Kiro",
          OPENCODE: "OpenCode",
        },
        locale: "zh-CN",
        value,
      }),
    localizeWorkerType: (value: string) =>
      localizeEnumLabel({
        category: "kind",
        labels: {
          HOSTED_WORKER: "托管 worker",
          MODEL_WORKER: "模型 worker",
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
