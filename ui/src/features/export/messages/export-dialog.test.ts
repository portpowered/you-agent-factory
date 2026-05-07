import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  exportDialogMessagesByLocale,
  getExportDialogMessages,
} from "./export-dialog";

describe("getExportDialogMessages", () => {
  it("supports the expected export dialog locales", () => {
    expect(Object.keys(exportDialogMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Export factory"],
    ["zh", "导出工厂"],
    ["ko", "팩토리 내보내기"],
    ["ja", "ファクトリーをエクスポート"],
  ] as const)("resolves %s catalog copy", (locale, expectedTitle) => {
    expect(getExportDialogMessages(locale).title).toBe(expectedTitle);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getExportDialogMessages("en");

    expect(getExportDialogMessages(undefined).title).toBe(
      defaultMessages.title,
    );
    expect(getExportDialogMessages("fr").title).toBe(defaultMessages.title);
  });

  it("keeps interpolation-based export copy available through the resolved locale catalog", () => {
    const messages = getExportDialogMessages("ja");

    expect(messages.successMessage("factory-aurora.png")).toContain(
      "factory-aurora.png",
    );
    expect(messages.selectedImageLabel("cover.png")).toContain("cover.png");
    expect(messages.triggerLabel).toBe("PNG をエクスポート");
  });
});
