import { describe, expect, it } from "bun:test";

import { validateRequiredLocaleMessages } from "./messages";
import {
  enumLocalizationContract,
  enumLocalizationMessagesByLocale,
  getEnumLocalizationMessages,
  getLocalizedEnumCategoryLabel,
  localizeEnumLabel,
} from "./enum-localization";

describe("enumLocalizationContract", () => {
  it("defines the customer-visible enum categories in scope", () => {
    expect(enumLocalizationContract).toMatchObject({
      categoriesInScope: [
        "status",
        "outcome",
        "kind",
        "type",
        "relation",
        "failureFamily",
      ],
      fallbackBehavior: "localized-unknown-with-raw-value",
    });
  });

  it("keeps non-product data values explicitly out of localization scope", () => {
    expect(enumLocalizationContract.exclusions).toEqual(
      expect.arrayContaining([
        "provider brand names",
        "model identifiers",
        "user-authored names",
        "IDs",
        "raw payload values",
      ]),
    );
  });
});

describe("getEnumLocalizationMessages", () => {
  it("returns required locale labels for English and zh-CN", () => {
    expect(getLocalizedEnumCategoryLabel("status", "en")).toBe("status");
    expect(getLocalizedEnumCategoryLabel("status", "zh-CN")).toBe("状态");
  });

  it("falls back to English when the locale is unsupported", () => {
    expect(getEnumLocalizationMessages("fr-FR").categories.outcome).toBe(
      "outcome",
    );
  });
});

describe("localizeEnumLabel", () => {
  const outcomeLabels = {
    category: "outcome" as const,
    labels: {
      ACCEPTED: "Accepted",
      FAILED: "Failed",
    },
  };

  it("resolves known enum labels in English", () => {
    expect(
      localizeEnumLabel({
        ...outcomeLabels,
        locale: "en",
        value: "ACCEPTED",
      }),
    ).toBe("Accepted");
  });

  it("resolves known enum labels in zh-CN when the owning surface supplies localized labels", () => {
    expect(
      localizeEnumLabel({
        category: "outcome",
        labels: {
          ACCEPTED: "已接受",
          FAILED: "失败",
        },
        locale: "zh-CN",
        value: "FAILED",
      }),
    ).toBe("失败");
  });

  it("renders a localized unknown wrapper while preserving the raw enum value", () => {
    expect(
      localizeEnumLabel({
        ...outcomeLabels,
        locale: "en",
        value: "WAITING_ON_REVIEW",
      }),
    ).toBe("Unknown outcome: WAITING_ON_REVIEW");

    expect(
      localizeEnumLabel({
        ...outcomeLabels,
        locale: "zh-CN",
        value: "WAITING_ON_REVIEW",
      }),
    ).toBe("未知结果：WAITING_ON_REVIEW");
  });
});

describe("enumLocalizationMessagesByLocale", () => {
  it("includes complete required locale coverage", () => {
    expect(validateRequiredLocaleMessages(enumLocalizationMessagesByLocale)).toEqual(
      [],
    );
  });
});
