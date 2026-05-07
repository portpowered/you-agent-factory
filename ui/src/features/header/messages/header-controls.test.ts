import { SUPPORTED_LOCALES } from "../../../i18n";
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
    ["zh", "仪表板概览"],
    ["ko", "대시보드 요약"],
    ["ja", "ダッシュボードの概要"],
  ] as const)("resolves %s catalog copy", (locale, expectedSummaryLabel) => {
    expect(getHeaderControlsMessages(locale).dashboardSummaryLabel).toBe(
      expectedSummaryLabel,
    );
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

  it.each(["en", "ja", "ko", "zh"] as const)(
    "keeps tick-status templates and stream labels available for %s",
    (locale) => {
      const messages = getHeaderControlsMessages(locale);

      expect(messages.currentTickStatusTemplate).toContain(
        HEADER_CURRENT_TICK_TOKEN,
      );
      expect(messages.currentTickStatusTemplate).toContain(
        HEADER_MAX_TICK_TOKEN,
      );
      expect(messages.streamStatusLiveLabel).toBeTruthy();
      expect(messages.streamStatusConnectingLabel).toBeTruthy();
      expect(messages.streamStatusOfflineLabel).toBeTruthy();
      expect(messages.returnToCurrentTickLabel).toBeTruthy();
      expect(messages.waitingForMoreTicks).toBeTruthy();
    },
  );
});
