import { SUPPORTED_LOCALES } from "../../../i18n";
import { getColorPaletteOptions } from "./color-palette-options";
import {
  getHeaderControlsMessages,
  HEADER_CURRENT_TICK_TOKEN,
  HEADER_MAX_TICK_TOKEN,
  headerControlsMessagesByLocale,
} from "./header-controls";

describe("getHeaderControlsMessages", () => {
  it("supports the expected header-control locales", () => {
    expect(Object.keys(headerControlsMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "dashboard summary"],
    ["zh-CN", "仪表板概览"],
    ["ko", "대시보드 요약"],
    ["ja", "ダッシュボードの概要"],
  ] as const)("resolves %s catalog copy", (locale, expectedSummaryLabel) => {
    expect(getHeaderControlsMessages(locale).dashboardSummaryLabel).toBe(
      expectedSummaryLabel,
    );
  });

  it.each([
    ["en", "Language"],
    ["zh-CN", "语言"],
    ["ko", "언어"],
    ["ja", "言語"],
  ] as const)(
    "keeps language-switcher labels available for %s",
    (locale, expectedLabel) => {
      const messages = getHeaderControlsMessages(locale);

      expect(messages.languageLabel).toBe(expectedLabel);
      expect(messages.languageMenuButtonLabel).toBeTruthy();
    },
  );

  it.each([
    ["en", "Color palette"],
    ["zh-CN", "调色板"],
    ["ko", "색상 팔레트"],
    ["ja", "カラーパレット"],
  ] as const)(
    "keeps palette-switcher labels available for %s",
    (locale, expectedLabel) => {
      const messages = getHeaderControlsMessages(locale);

      expect(messages.paletteLabel).toBe(expectedLabel);
      expect(messages.paletteMenuButtonLabel).toBeTruthy();
    },
  );

  it("keeps five localized palette option labels for English", () => {
    expect(getColorPaletteOptions("en").map((option) => option.label)).toEqual([
      "Factory Dark",
      "Factory Light",
      "Material Baseline",
      "Slate",
      "Olive",
    ]);
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getHeaderControlsMessages("en");

    expect(getHeaderControlsMessages(undefined).sliderLabel).toBe(
      defaultMessages.sliderLabel,
    );
    expect(getHeaderControlsMessages("fr").sliderLabel).toBe(
      defaultMessages.sliderLabel,
    );
  });

  it.each(["en", "ja", "ko", "zh-CN"] as const)(
    "keeps compact tick-status templates and stream labels available for %s",
    (locale) => {
      const messages = getHeaderControlsMessages(locale);

      expect(messages.currentTickStatusTemplate).toContain(
        HEADER_CURRENT_TICK_TOKEN,
      );
      expect(messages.currentTickStatusTemplate).toContain(
        HEADER_MAX_TICK_TOKEN,
      );
      expect(messages.currentTickStatusTemplate).toBe(
        `${HEADER_CURRENT_TICK_TOKEN}/${HEADER_MAX_TICK_TOKEN}`,
      );
      expect(messages.streamStatusLiveLabel).toBeTruthy();
      expect(messages.streamStatusConnectingLabel).toBeTruthy();
      expect(messages.streamStatusOfflineLabel).toBeTruthy();
      expect(messages.pauseSessionStreamLabelTemplate).toContain(
        "{{sessionLabel}}",
      );
      expect(messages.resumeSessionStreamLabelTemplate).toContain(
        "{{sessionLabel}}",
      );
      expect(messages.returnToCurrentTickLabel).toBeTruthy();
      expect(messages.waitingForMoreTicks).toBeTruthy();
      expect(messages.globalHeaderActionsLabel).toBeTruthy();
      expect(messages.languageLabel).toBeTruthy();
      expect(messages.languageMenuButtonLabel).toBeTruthy();
    },
  );
});
