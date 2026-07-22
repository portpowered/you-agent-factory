import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface CliManifestMessages {
  contractFailure: (path: string, detail: string) => string;
  requiredField: (field: string) => string;
}

const cliManifestMessagesByLocale = {
  en: {
    contractFailure: (path, detail) =>
      `Expected ${path} to satisfy the CLI manifest contract: ${detail}.`,
    requiredField: (field) => `Expected required field ${field}.`,
  },
  "zh-CN": {
    contractFailure: (path, detail) =>
      `${path} 应符合 CLI 清单契约：${detail}。`,
    requiredField: (field) => `缺少必填字段 ${field}。`,
  },
} satisfies LocalizedMessageCatalog<CliManifestMessages>;

export function getCliManifestMessages(locale?: string | null) {
  return resolveLocalizedMessages(cliManifestMessagesByLocale, locale);
}
