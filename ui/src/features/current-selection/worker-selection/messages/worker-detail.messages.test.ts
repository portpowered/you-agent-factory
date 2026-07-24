import { describe, expect, it } from "vitest";

import { validateRequiredLocaleMessages } from "../../../../i18n";
import {
  getWorkerDetailMessages,
  workerDetailMessagesByLocale,
} from "./worker-detail";

const locales = ["en", "ja", "ko", "zh-CN"] as const;

describe("workerDetailMessagesByLocale", () => {
  it("includes complete required locale coverage", () => {
    expect(
      validateRequiredLocaleMessages(workerDetailMessagesByLocale),
    ).toEqual([]);
  });
});

describe("getWorkerDetailMessages", () => {
  it.each(locales)(
    "returns non-empty worker editor editable copy for $locale",
    (locale) => {
      const messages = getWorkerDetailMessages(locale);

      expect(messages.editableConfigurationHeading.length).toBeGreaterThan(0);
      expect(messages.editableConfigurationSaveAction.length).toBeGreaterThan(
        0,
      );
      expect(
        messages.editableConfigurationValidationStatus.length,
      ).toBeGreaterThan(0);
      expect(messages.nameFieldLabel.length).toBeGreaterThan(0);
      expect(messages.typeFieldLabel.length).toBeGreaterThan(0);
      expect(messages.timeoutFieldLabel.length).toBeGreaterThan(0);
      expect(messages.stopTokenFieldLabel.length).toBeGreaterThan(0);
      expect(messages.modelProviderLabel.length).toBeGreaterThan(0);
      expect(messages.commandFieldLabel.length).toBeGreaterThan(0);
      expect(messages.providerFieldLabel.length).toBeGreaterThan(0);
      expect(messages.linearTeamIdsFieldLabel.length).toBeGreaterThan(0);
      expect(messages.skipPermissionsFieldLabel.length).toBeGreaterThan(0);
      expect(messages.editableConfigurationNameRequired.length).toBeGreaterThan(
        0,
      );
      expect(
        messages.editableConfigurationSaveSuccess("reviewer").length,
      ).toBeGreaterThan(0);
      expect(
        messages.editableConfigurationOverwriteWarning("worker name").length,
      ).toBeGreaterThan(0);
      expect(
        messages.editableConfigurationServerFieldChangedHint.length,
      ).toBeGreaterThan(0);
      expect(
        messages.localizeWorkerType("MODEL_WORKER").length,
      ).toBeGreaterThan(0);
      expect(messages.localizeModelProvider("CURSOR").length).toBeGreaterThan(
        0,
      );
      expect(messages.localizeTimeoutUnit("s").length).toBeGreaterThan(0);
    },
  );
});
