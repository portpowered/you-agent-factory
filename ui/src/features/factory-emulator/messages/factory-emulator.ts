import {
  type LocalizedMessageCatalog,
  resolveLocalizedMessages,
} from "../../../i18n";

export interface FactoryEmulatorMessages {
  submission: {
    blank: string;
    wrongWorkType: string;
    unavailable: {
      "ambiguous-default": string;
      closed: string;
      error: string;
      history: string;
      invalid: string;
      loading: string;
      "no-default": string;
    };
  };
}

const factoryEmulatorMessagesByLocale = {
  en: {
    submission: {
      blank: "Enter nonblank text to submit.",
      wrongWorkType: "Submit to the eligible default Work Type.",
      unavailable: {
        "ambiguous-default":
          "Configure exactly one eligible default Work Type.",
        closed: "The emulator session is closed.",
        error:
          "Retry or restart the invalid emulator session before submitting.",
        history: "Return to the current tick before submitting.",
        invalid:
          "Retry or restart the invalid emulator session before submitting.",
        loading: "Start the emulator before submitting.",
        "no-default": "No eligible default Work Type is available.",
      },
    },
  },
  "zh-CN": {
    submission: {
      blank: "请输入非空文本后再提交。",
      wrongWorkType: "请提交到符合条件的默认工作类型。",
      unavailable: {
        "ambiguous-default": "请仅配置一个符合条件的默认工作类型。",
        closed: "模拟器会话已关闭。",
        error: "请先重试或重新启动无效的模拟器会话。",
        history: "请返回当前逻辑时点后再提交。",
        invalid: "请先重试或重新启动无效的模拟器会话。",
        loading: "请先启动模拟器。",
        "no-default": "没有符合条件的默认工作类型。",
      },
    },
  },
} satisfies LocalizedMessageCatalog<FactoryEmulatorMessages>;

export function getFactoryEmulatorMessages(locale?: string | null) {
  return resolveLocalizedMessages(factoryEmulatorMessagesByLocale, locale);
}
