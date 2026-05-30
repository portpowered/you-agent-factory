import { describe, expect, it } from "vitest";
import { submitWorkItemRowTypeLabel } from "./submit-work-item-type-label";

describe("submitWorkItemRowTypeLabel", () => {
  it("uses shared work-content labels for canonical part types", () => {
    expect(submitWorkItemRowTypeLabel("text")).toBe("Text");
    expect(submitWorkItemRowTypeLabel("audio")).toBe("Audio");
    expect(submitWorkItemRowTypeLabel("image")).toBe("Image");
  });

  it("uses submit-work catalog labels for staging-only part types", () => {
    expect(submitWorkItemRowTypeLabel("document")).toBe("Document");
    expect(submitWorkItemRowTypeLabel("video")).toBe("Video");
  });
});
