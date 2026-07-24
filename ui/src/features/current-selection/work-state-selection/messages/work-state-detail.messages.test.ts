import { describe, expect, it } from "vitest";

import { getWorkStateDetailMessages } from "./work-state-detail";

const locales = ["en", "ja", "ko", "zh-CN"] as const;

describe("getWorkStateDetailMessages", () => {
  it.each(locales)(
    "returns non-empty editable field copy for $locale",
    (locale) => {
      const messages = getWorkStateDetailMessages(locale);

      expect(messages.nameFieldLabel.length).toBeGreaterThan(0);
      expect(messages.typeFieldLabel.length).toBeGreaterThan(0);
      expect(messages.editableConfigurationSaveAction.length).toBeGreaterThan(
        0,
      );
      expect(messages.editableConfigurationNameRequired.length).toBeGreaterThan(
        0,
      );
      expect(
        messages.localizeWorkStateType("PROCESSING").length,
      ).toBeGreaterThan(0);
      expect(
        messages.editableConfigurationNameDuplicate("queued").length,
      ).toBeGreaterThan(0);
    },
  );
});
