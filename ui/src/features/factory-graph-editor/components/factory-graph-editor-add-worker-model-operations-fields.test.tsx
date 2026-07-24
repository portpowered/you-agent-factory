import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { createEmptyFactoryGraphAddModelOperationDraft } from "../lib/factory-graph-add-model-operation-draft";
import { FactoryGraphEditorAddWorkerModelOperationsFields } from "./factory-graph-editor-add-worker-model-operations-fields";
import { buildOperationDraft } from "./factory-graph-editor-add-worker-model-operations-fields.test-helpers";

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("FactoryGraphEditorAddWorkerModelOperationsFields", () => {
  it("renders validation errors and edits operation contracts", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const operations = [buildOperationDraft()];

    render(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        errors={{
          byIndex: {
            0: {
              inputSlots: {
                0: { contentTypes: "Select at least one content type." },
              },
              name: "Operation names must be uppercase letters, digits, or underscores.",
            },
          },
          summary:
            "Fix model operation contract errors before adding this worker.",
        }}
        onChange={onChange}
        operations={operations}
      />,
    );

    expect(
      screen.getByText(
        "Fix model operation contract errors before adding this worker.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Operation names must be uppercase letters, digits, or underscores.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Select at least one content type."),
    ).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Operation name"), {
      target: { value: "STT" },
    });
    expect(onChange).toHaveBeenCalledWith([
      expect.objectContaining({ name: "STT" }),
    ]);

    await user.click(screen.getByRole("button", { name: "Add input slot" }));
    expect(onChange).toHaveBeenLastCalledWith([
      expect.objectContaining({
        inputs: expect.arrayContaining([
          expect.objectContaining({ name: "text" }),
          expect.objectContaining({ name: "", required: true }),
        ]),
      }),
    ]);

    await user.click(screen.getByRole("button", { name: "Add output slot" }));
    expect(onChange).toHaveBeenLastCalledWith([
      expect.objectContaining({
        outputs: expect.arrayContaining([
          expect.objectContaining({ name: "audio" }),
          expect.objectContaining({ name: "", required: false }),
        ]),
      }),
    ]);

    const requiredCheckbox = screen.getByLabelText("Required input slot");
    await user.click(requiredCheckbox);
    expect(onChange).toHaveBeenCalledWith([
      expect.objectContaining({
        inputs: [expect.objectContaining({ required: false })],
      }),
    ]);

    await user.click(screen.getAllByLabelText("JSON")[0]);
    expect(onChange).toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Remove operation" }));
    expect(onChange).toHaveBeenLastCalledWith([]);

    onChange.mockClear();
    await user.click(screen.getByRole("button", { name: "Add operation" }));
    expect(onChange).toHaveBeenLastCalledWith([
      operations[0],
      createEmptyFactoryGraphAddModelOperationDraft(),
    ]);
  });

  it("removes extra slots when more than one slot exists", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const operation = buildOperationDraft();
    operation.inputs.push({
      contentTypes: ["JSON"],
      name: "voice",
      required: false,
    });

    render(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        onChange={onChange}
        operations={[operation]}
      />,
    );

    const removeSlotButtons = screen.getAllByRole("button", {
      name: "Remove slot",
    });
    expect(removeSlotButtons[0]).not.toBeDisabled();

    await user.click(removeSlotButtons[0]);
    expect(onChange).toHaveBeenCalledWith([
      expect.objectContaining({
        inputs: [expect.objectContaining({ name: "voice" })],
      }),
    ]);
  });
});
