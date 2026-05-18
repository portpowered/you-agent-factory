import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  getWorkstationDetailMessages,
  workstationDetailMessagesByLocale,
} from "./workstation-detail";

describe("getWorkstationDetailMessages", () => {
  it("supports the expected workstation detail locales", () => {
    expect(Object.keys(workstationDetailMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Workstation summary"],
    ["zh", "工作站摘要"],
    ["ko", "워크스테이션 요약"],
    ["ja", "ワークステーション概要"],
  ] as const)("resolves %s catalog copy", (locale, expectedSummaryHeading) => {
    expect(getWorkstationDetailMessages(locale).summaryHeading).toBe(
      expectedSummaryHeading,
    );
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getWorkstationDetailMessages("en");

    expect(getWorkstationDetailMessages(undefined).summaryHeading).toBe(
      defaultMessages.summaryHeading,
    );
    expect(getWorkstationDetailMessages("fr").summaryHeading).toBe(
      defaultMessages.summaryHeading,
    );
  });

  it("keeps interpolation helpers available across the workstation detail catalog", () => {
    const messages = getWorkstationDetailMessages("ja");

    expect(messages.historyRequestCountLabel(2)).toContain("2");
    expect(messages.requestStatusStartedAgo("4s")).toContain("4s");
    expect(messages.providerSummary("codex", "gpt-5.4")).toContain("gpt-5.4");
    expect(messages.scriptCommandSummary("script-tool")).toContain(
      "script-tool",
    );
    expect(messages.selectedRequestLabel("dispatch-review-active")).toContain(
      "dispatch-review-active",
    );
    expect(messages.openNamedWorkItemAction("Rejected Story")).toContain(
      "Rejected Story",
    );
    expect(messages.selectWorkItemLabel("Rejected Story")).toContain(
      "Rejected Story",
    );
    expect(
      messages.selectRequestLabel(
        "Rejected Story",
        "dispatch-review-rejected",
      ),
    ).toContain("dispatch-review-rejected");
    expect(
      messages.selectWorkstationRequestLabel("dispatch-review-active"),
    ).toContain("dispatch-review-active");
    expect(messages.workDetailsUnavailable("dispatch-review-active")).toContain(
      "dispatch-review-active",
    );
  });

  it.each([
    "ko",
    "zh",
  ] as const)("keeps %s request and run count helpers available", (locale) => {
    const messages = getWorkstationDetailMessages(locale);

    expect(messages.historyRequestCountLabel(1)).toContain("1");
    expect(messages.historyRunCountLabel(3)).toContain("3");
    expect(
      messages.requestDetailsUnavailable("dispatch-review-active"),
    ).toContain("dispatch-review-active");
    expect(messages.openNamedWorkItemAction("Retry Story")).toContain(
      "Retry Story",
    );
  });

  it.each([
    "en",
    "ja",
    "ko",
    "zh",
  ] as const)(
    "keeps all %s workstation-detail helper messages callable",
    (locale) => {
      const messages = getWorkstationDetailMessages(locale);

      expect(
        messages.editableConfigurationOverwriteWarning("prompt, model"),
      ).toContain("prompt");
      expect(
        messages.editableConfigurationSaveConflictConfirmationDescription(
          "template",
        ),
      ).toContain("template");
      expect(messages.historyRequestCountLabel(4)).toContain("4");
      expect(messages.historyRunCountLabel(5)).toContain("5");
      expect(messages.providerSummary("codex", null)).toContain("codex");
      expect(messages.providerSummary("codex", "gpt-5.4")).toContain(
        "gpt-5.4",
      );
      expect(
        messages.requestDetailsUnavailable("dispatch-review-history"),
      ).toContain("dispatch-review-history");
      expect(messages.requestStatusStartedAgo("9s")).toContain("9s");
      expect(messages.scriptCommandSummary("script-tool")).toContain(
        "script-tool",
      );
      expect(
        messages.selectRequestLabel(
          "Review Story",
          "dispatch-review-history",
        ),
      ).toContain("dispatch-review-history");
      expect(messages.selectWorkItemLabel("Review Story")).toContain(
        "Review Story",
      );
      expect(
        messages.selectWorkstationRequestLabel("dispatch-review-history"),
      ).toContain("dispatch-review-history");
      expect(messages.selectedRequestLabel("dispatch-review-history")).toContain(
        "dispatch-review-history",
      );
      expect(messages.openNamedWorkItemAction("Review Story")).toContain(
        "Review Story",
      );
      expect(messages.workDetailsUnavailable("dispatch-review-history")).toContain(
        "dispatch-review-history",
      );
    },
  );
});
