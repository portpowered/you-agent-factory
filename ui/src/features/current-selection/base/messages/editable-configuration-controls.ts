import {
  type LocalizedMessages,
  resolveLocalizedMessages,
} from "../../../../i18n";

export interface EditableConfigurationControlsMessages {
  discardLocalChangesAction: string;
}

const editableConfigurationControlsMessagesByLocale = {
  en: {
    discardLocalChangesAction: "Discard local changes",
  },
  ja: {
    discardLocalChangesAction: "ローカル変更を破棄",
  },
  ko: {
    discardLocalChangesAction: "로컬 변경 사항 취소",
  },
  "zh-CN": {
    discardLocalChangesAction: "放弃本地更改",
  },
} satisfies LocalizedMessages<EditableConfigurationControlsMessages>;

export function getEditableConfigurationControlsMessages(
  locale?: string,
): EditableConfigurationControlsMessages {
  return resolveLocalizedMessages(
    editableConfigurationControlsMessagesByLocale,
    locale,
  );
}

export { editableConfigurationControlsMessagesByLocale };
