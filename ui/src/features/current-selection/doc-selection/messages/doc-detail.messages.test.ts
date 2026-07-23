import { describe, expect, it } from "vitest";

import { getDocDetailMessages } from "./doc-detail";

const locales = ["en", "zh-CN"] as const;

describe("getDocDetailMessages", () => {
  it.each(locales)(
    "returns non-empty editable doc copy for $locale",
    (locale) => {
      const messages = getDocDetailMessages(locale);

      expect(messages.editableConfigurationHeading.length).toBeGreaterThan(0);
      expect(messages.editableConfigurationSaveAction.length).toBeGreaterThan(
        0,
      );
      expect(
        messages.editableConfigurationFileNameRequired.length,
      ).toBeGreaterThan(0);
      expect(
        messages.editableConfigurationSaveSuccess("guide.md").length,
      ).toBeGreaterThan(0);
      expect(
        messages.editableConfigurationOverwriteWarning("file name").length,
      ).toBeGreaterThan(0);
      expect(
        messages.editableConfigurationFileNameDuplicate("guide.md").length,
      ).toBeGreaterThan(0);
    },
  );
});
