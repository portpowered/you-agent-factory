import type { EditableResourceDraft } from "../../../current-factory-definition/lib/resource-editable-values";
import { getResourceDetailMessages } from "../messages/resource-detail";
import {
  formatEditableResourceOverwriteFieldLabels,
  resolveEditableResourceOverwriteFields,
} from "./editable-resource-overwrite-fields";

function buildDraft(
  overrides: Partial<EditableResourceDraft> = {},
): EditableResourceDraft {
  return {
    backend: "llama.cpp",
    capacityText: "1",
    loadPolicy: "ON_DEMAND",
    model: "OMNIVOICE_Q4_K_M",
    name: "voice-model",
    provider: "anthropic",
    type: "MODEL",
    ...overrides,
  };
}

describe("resolveEditableResourceOverwriteFields", () => {
  it("returns no fields when session, draft, and latest match", () => {
    const draft = buildDraft();

    expect(
      resolveEditableResourceOverwriteFields(draft, draft, draft),
    ).toEqual([]);
  });

  it("flags resource fields that diverged locally and on the server", () => {
    const sessionStart = buildDraft({
      backend: "llama.cpp",
      capacityText: "1",
      model: "OMNIVOICE_Q8_0",
    });
    const current = buildDraft({
      backend: "vllm",
      capacityText: "2",
      model: "OMNIVOICE_Q8_0",
    });
    const latest = buildDraft({
      backend: "tensorrt",
      capacityText: "3",
      model: "OMNIVOICE_Q4_K_M",
    });

    expect(
      resolveEditableResourceOverwriteFields(sessionStart, current, latest),
    ).toEqual(expect.arrayContaining(["capacity", "model", "backend"]));
  });

  it("flags provider quota fields when both sides changed", () => {
    const sessionStart = buildDraft({
      provider: "anthropic",
      type: "PROVIDER_QUOTA",
    });
    const current = buildDraft({
      provider: "openai",
      type: "PROVIDER_QUOTA",
    });
    const latest = buildDraft({
      provider: "google",
      type: "INVOCATION_SLOT",
    });

    expect(
      resolveEditableResourceOverwriteFields(sessionStart, current, latest),
    ).toEqual(expect.arrayContaining(["provider", "type"]));
  });

  it("flags fields when session start and draft both differ from the latest server draft", () => {
    const sessionStart = buildDraft({ name: "voice-model" });
    const current = buildDraft({ name: "voice-model" });
    const latest = buildDraft({ name: "voice-runtime" });

    expect(
      resolveEditableResourceOverwriteFields(sessionStart, current, latest),
    ).toEqual(["name"]);
  });
});

describe("formatEditableResourceOverwriteFieldLabels", () => {
  it("formats overwrite field labels for the overwrite warning banner", () => {
    const messages = getResourceDetailMessages();
    const formatted = formatEditableResourceOverwriteFieldLabels(
      ["type", "capacity", "model", "backend", "loadPolicy", "provider", "name"],
      messages,
    );

    expect(formatted).toContain(messages.typeFieldLabel.toLowerCase());
    expect(formatted).toContain(messages.capacityFieldLabel.toLowerCase());
    expect(formatted).toContain(messages.modelFieldLabel.toLowerCase());
    expect(formatted).toContain(messages.providerFieldLabel.toLowerCase());
    expect(formatted).toContain(messages.nameFieldLabel.toLowerCase());
  });
});
