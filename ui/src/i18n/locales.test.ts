import { describe, expect, it } from "vitest";

import { DEFAULT_LOCALE, resolveSupportedLocale, SUPPORTED_LOCALES } from "./locales";

describe("resolveSupportedLocale", () => {
  it("supports the expected locale set", () => {
    expect(SUPPORTED_LOCALES).toEqual(["en", "zh", "ko", "ja"]);
  });

  it.each([
    ["en", "en"],
    ["zh", "zh"],
    ["ko", "ko"],
    ["ja", "ja"],
    ["ja-JP", "ja"],
    ["ko_KR", "ko"],
    ["ZH-hant", "zh"],
  ] as const)("resolves %s to %s", (locale, expected) => {
    expect(resolveSupportedLocale(locale)).toBe(expected);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    expect(resolveSupportedLocale(undefined)).toBe(DEFAULT_LOCALE);
    expect(resolveSupportedLocale(null)).toBe(DEFAULT_LOCALE);
    expect(resolveSupportedLocale("fr-FR")).toBe(DEFAULT_LOCALE);
  });
});
