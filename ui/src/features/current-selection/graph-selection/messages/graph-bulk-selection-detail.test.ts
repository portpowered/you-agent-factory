import { describe, expect, it } from "vitest";

import {
  getGraphBulkSelectionDetailMessages,
  graphBulkSelectionKindLabel,
} from "./graph-bulk-selection-detail";

describe("graph-bulk-selection-detail messages", () => {
  it("resolves localized labels for known graph item kinds", () => {
    const messages = getGraphBulkSelectionDetailMessages("en");

    expect(graphBulkSelectionKindLabel(messages, "workstation")).toBe(
      "Workstations",
    );
    expect(graphBulkSelectionKindLabel(messages, "edge")).toBe("Edges");
  });

  it("falls back to the unknown kind label for unsupported kinds", () => {
    const messages = getGraphBulkSelectionDetailMessages("en");

    expect(
      graphBulkSelectionKindLabel(
        messages,
        "unsupported-kind" as "workstation",
      ),
    ).toBe("Other graph items");
  });

  it("resolves localized message bundles for supported locales", () => {
    expect(getGraphBulkSelectionDetailMessages("zh-CN").heading).toBe(
      "已选择多个图项",
    );
    expect(getGraphBulkSelectionDetailMessages("ja").edgeKindLabel).toBe(
      "エッジ",
    );
  });
});
