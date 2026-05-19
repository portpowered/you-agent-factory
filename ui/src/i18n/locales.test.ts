import {
  DEFAULT_LOCALE,
  isSupportedLocale,
  resolveSupportedLocale,
  SUPPORTED_LOCALES,
} from "./locales";

describe("website locale policy", () => {
  it("defines English as the default fallback locale and zh-CN as the Mandarin locale", () => {
    expect(DEFAULT_LOCALE).toBe("en");
    expect(SUPPORTED_LOCALES).toContain("en");
    expect(SUPPORTED_LOCALES).toContain("zh-CN");
  });

  it("recognizes canonical supported locales", () => {
    expect(isSupportedLocale("en")).toBe(true);
    expect(isSupportedLocale("zh-CN")).toBe(true);
  });

  it.each([
    [undefined, "en"],
    [null, "en"],
    ["", "en"],
    ["   ", "en"],
    ["fr", "en"],
    ["not a locale", "en"],
  ] as const)("resolves missing, malformed, or unsupported locale %s to English", (locale, expected) => {
    expect(resolveSupportedLocale(locale)).toBe(expected);
  });

  it.each([
    ["en", "en"],
    ["EN", "en"],
    ["en-US", "en"],
    ["ja-JP", "ja"],
    ["ko-KR", "ko"],
    ["zh-CN", "zh-CN"],
    ["zh_CN", "zh-CN"],
    ["ZH-cn", "zh-CN"],
  ] as const)("resolves canonical locale input %s", (locale, expected) => {
    expect(resolveSupportedLocale(locale)).toBe(expected);
  });

  it.each([
    ["zh", "zh-CN"],
    ["zh-Hans", "zh-CN"],
    ["ZH-HANS", "zh-CN"],
    ["zh-hans", "zh-CN"],
    ["zh-Hans-CN", "zh-CN"],
  ] as const)("resolves Mandarin alias %s to zh-CN", (locale, expected) => {
    expect(resolveSupportedLocale(locale)).toBe(expected);
  });
});
