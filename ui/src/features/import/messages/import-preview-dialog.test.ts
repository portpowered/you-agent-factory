import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getImportPreviewDialogMessages,
  IMPORT_PREVIEW_FACTORY_NAME_TOKEN,
  importPreviewDialogMessagesByLocale,
} from "./import-preview-dialog";

describe("getImportPreviewDialogMessages", () => {
  it("supports the expected import preview locales", () => {
    expect(Object.keys(importPreviewDialogMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Review factory import"],
    ["zh", "检查工厂导入"],
    ["ko", "팩토리 가져오기 검토"],
    ["ja", "ファクトリーのインポートを確認"],
  ] as const)("resolves %s catalog copy", (locale, expectedTitle) => {
    expect(getImportPreviewDialogMessages(locale).title).toBe(expectedTitle);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getImportPreviewDialogMessages("en");

    expect(getImportPreviewDialogMessages(undefined).title).toBe(defaultMessages.title);
    expect(getImportPreviewDialogMessages("fr").title).toBe(defaultMessages.title);
  });

  it("keeps interpolation and mapped activation copy available through the resolved locale catalog", () => {
    const messages = getImportPreviewDialogMessages("zh");

    expect(messages.descriptionTemplate).toContain(IMPORT_PREVIEW_FACTORY_NAME_TOKEN);
    expect(messages.previewImageAlt("Dropped Factory")).toContain("Dropped Factory");
    expect(messages.errorByCode.NETWORK_ERROR).toBe(
      "仪表板无法连接到启用 API。请在连接恢复后重试。",
    );
  });
});
