import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  dashboardSessionControlsMessagesByLocale,
  getDashboardSessionControlsMessages,
} from "./dashboard-session-controls";

describe("getDashboardSessionControlsMessages", () => {
  it("supports every dashboard locale", () => {
    expect(
      Object.keys(dashboardSessionControlsMessagesByLocale).sort(),
    ).toEqual([...SUPPORTED_LOCALES].sort());
  });

  it.each(["en", "ja", "ko", "zh-CN"] as const)(
    "keeps session state copy and labels available for %s",
    (locale) => {
      const messages = getDashboardSessionControlsMessages(locale);

      expect(messages.factoryLifecyclePausedLabel).toBeTruthy();
      expect(messages.factoryLifecycleRunningLabel).toBeTruthy();
      expect(messages.liveDashboardUpdatesPausedLabel).toBeTruthy();
      expect(messages.pauseLiveUpdatesLabelTemplate).toContain(
        "{{sessionLabel}}",
      );
      expect(messages.resumeLiveUpdatesLabelTemplate).toContain(
        "{{sessionLabel}}",
      );
      expect(messages.timelineModeHistoricalLabel).toBeTruthy();
      expect(messages.timelineModeLabel).toBeTruthy();
      expect(messages.timelineModeLiveLabel).toBeTruthy();
    },
  );

  it("falls back to English for missing or unsupported locales", () => {
    const defaultMessages = getDashboardSessionControlsMessages("en");

    expect(
      getDashboardSessionControlsMessages(undefined).timelineModeLabel,
    ).toBe(defaultMessages.timelineModeLabel);
    expect(getDashboardSessionControlsMessages("fr").timelineModeLabel).toBe(
      defaultMessages.timelineModeLabel,
    );
  });
});
