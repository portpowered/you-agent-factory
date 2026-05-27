import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getSharedPrimitiveMessages,
  sharedPrimitiveMessagesByLocale,
} from "./shared-primitives";

describe("getSharedPrimitiveMessages", () => {
  it("supports the expected shared primitive locales", () => {
    expect(Object.keys(sharedPrimitiveMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Shared UI primitives", "No rows available."],
    ["zh-CN", "共享 UI 基础组件", "没有可用行。"],
    ["ko", "공유 UI 프리미티브", "사용 가능한 행이 없습니다."],
    ["ja", "共有 UI プリミティブ", "行はありません。"],
  ] as const)("resolves %s catalog copy", (locale, title, emptyMessage) => {
    const messages = getSharedPrimitiveMessages(locale);

    expect(messages.showcaseTitle).toBe(title);
    expect(messages.emptyTableMessage).toBe(emptyMessage);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getSharedPrimitiveMessages("en");

    expect(getSharedPrimitiveMessages(undefined).dialogCloseLabel).toBe(
      defaultMessages.dialogCloseLabel,
    );
    expect(getSharedPrimitiveMessages("fr").dialogCloseLabel).toBe(
      defaultMessages.dialogCloseLabel,
    );
  });

  it("keeps the session-log path template placeholders available", () => {
    const messages = getSharedPrimitiveMessages("zh-CN");

    expect(messages.sessionLogPathTemplate).toContain("{{year}}");
    expect(messages.sessionLogPathTemplate).toContain("{{month}}");
    expect(messages.sessionLogPathTemplate).toContain("{{day}}");
    expect(messages.sessionLogPathTemplate).toContain("{{sessionID}}");
  });
});
