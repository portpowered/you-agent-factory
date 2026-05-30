import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import {
  formatEditableWorkerOverwriteFieldLabels,
  resolveEditableWorkerOverwriteFields,
} from "./editable-worker-overwrite-fields";

function buildDraft(
  overrides: Partial<EditableWorkerDraft> = {},
): EditableWorkerDraft {
  return {
    argsText: "arg-one",
    body: "body text",
    command: "run.sh",
    executorProvider: "SCRIPT_WRAP",
    model: "gpt-5.5",
    modelLocality: "CLOUD",
    modelProvider: "CURSOR",
    provider: "LINEAR",
    type: "MODEL_WORKER",
    ...overrides,
  };
}

describe("resolveEditableWorkerOverwriteFields", () => {
  it("returns no fields when session, draft, and latest match", () => {
    const draft = buildDraft();

    expect(
      resolveEditableWorkerOverwriteFields(draft, draft, draft),
    ).toEqual([]);
  });

  it("flags model worker fields that diverged locally and on the server", () => {
    const sessionStart = buildDraft({
      model: "gpt-4",
      modelProvider: "CURSOR",
      type: "MODEL_WORKER",
    });
    const current = buildDraft({
      model: "gpt-5",
      modelProvider: "CODEX",
      type: "MODEL_WORKER",
    });
    const latest = buildDraft({
      model: "gpt-5.5",
      modelProvider: "CLAUDE",
      type: "MODEL_WORKER",
    });

    expect(
      resolveEditableWorkerOverwriteFields(sessionStart, current, latest),
    ).toEqual(expect.arrayContaining(["model", "modelProvider"]));
  });

  it("flags script and hosted worker fields when both sides changed", () => {
    const sessionStart = buildDraft({
      argsText: "one",
      body: "old body",
      command: "old.sh",
      provider: "LINEAR",
      type: "SCRIPT_WORKER",
    });
    const current = buildDraft({
      argsText: "two",
      body: "new body",
      command: "new.sh",
      provider: "LINEAR",
      type: "HOSTED_WORKER",
    });
    const latest = buildDraft({
      argsText: "three",
      body: "server body",
      command: "server.sh",
      provider: null,
      type: "HOSTED_WORKER",
    });

    expect(
      resolveEditableWorkerOverwriteFields(sessionStart, current, latest),
    ).toEqual(
      expect.arrayContaining(["command", "args", "body", "provider"]),
    );
  });

  it("flags fields when session start and draft both differ from the latest server draft", () => {
    const sessionStart = buildDraft({ model: "gpt-4" });
    const current = buildDraft({ model: "gpt-4" });
    const latest = buildDraft({ model: "gpt-5.5" });

    expect(
      resolveEditableWorkerOverwriteFields(sessionStart, current, latest),
    ).toEqual(["model"]);
  });
});

describe("formatEditableWorkerOverwriteFieldLabels", () => {
  it("formats overwrite field labels for the overwrite warning banner", () => {
    const messages = getWorkerDetailMessages();
    const formatted = formatEditableWorkerOverwriteFieldLabels(
      [
        "type",
        "modelProvider",
        "model",
        "modelLocality",
        "executorProvider",
        "command",
        "args",
        "body",
        "provider",
      ],
      messages,
    );

    expect(formatted).toContain(messages.typeFieldLabel.toLowerCase());
    expect(formatted).toContain(messages.modelLabel.toLowerCase());
    expect(formatted).toContain(messages.commandFieldLabel.toLowerCase());
    expect(formatted).toContain(messages.providerFieldLabel.toLowerCase());
  });
});
