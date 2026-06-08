import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { expectStyledCheckbox } from "../../../testing/checkbox-test-helpers";
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

describe("FactoryGraphEditorAddWorkerModelOperationsFields styled checkboxes", () => {
  it("renders shared styled checkboxes for content types and required input slot", () => {
    render(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        onChange={vi.fn()}
        operations={[buildOperationDraft()]}
      />,
    );

    const requiredCheckbox = screen.getByLabelText("Required input slot");
    expectStyledCheckbox(requiredCheckbox);
    expect(requiredCheckbox.checked).toBe(true);

    const inputTextCheckbox = document.getElementById(
      "factory-graph-add-model-operation-input-slot-0-content-type-TEXT",
    );
    expect(inputTextCheckbox).toBeTruthy();
    expectStyledCheckbox(inputTextCheckbox as HTMLElement);
    expect((inputTextCheckbox as HTMLInputElement).checked).toBe(true);

    const outputAudioCheckbox = document.getElementById(
      "factory-graph-add-model-operation-output-slot-0-content-type-AUDIO",
    );
    expect(outputAudioCheckbox).toBeTruthy();
    expectStyledCheckbox(outputAudioCheckbox as HTMLElement);
    expect((outputAudioCheckbox as HTMLInputElement).checked).toBe(true);
  });

  it("toggles required input slot with Space while focused", async () => {
    const user = userEvent.setup();
    let operations = [buildOperationDraft()];
    const onChange = vi.fn((nextOperations) => {
      operations = nextOperations;
    });

    const { rerender } = render(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        onChange={onChange}
        operations={operations}
      />,
    );

    const requiredCheckbox = screen.getByLabelText("Required input slot");
    requiredCheckbox.focus();
    await user.keyboard(" ");

    rerender(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        onChange={onChange}
        operations={operations}
      />,
    );

    expect(onChange).toHaveBeenCalledWith([
      expect.objectContaining({
        inputs: [expect.objectContaining({ required: false })],
      }),
    ]);
    expect(requiredCheckbox.checked).toBe(false);
  });

  it("toggles content type selection from checkbox clicks", async () => {
    const user = userEvent.setup();
    let operations = [buildOperationDraft()];
    const onChange = vi.fn((nextOperations) => {
      operations = nextOperations;
    });

    const { rerender } = render(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        onChange={onChange}
        operations={operations}
      />,
    );

    const inputJsonCheckbox = document.getElementById(
      "factory-graph-add-model-operation-input-slot-0-content-type-JSON",
    );
    expect(inputJsonCheckbox).toBeTruthy();
    await user.click(inputJsonCheckbox as HTMLInputElement);

    rerender(
      <FactoryGraphEditorAddWorkerModelOperationsFields
        onChange={onChange}
        operations={operations}
      />,
    );

    expect(onChange).toHaveBeenCalledWith([
      expect.objectContaining({
        inputs: [
          expect.objectContaining({
            contentTypes: expect.arrayContaining(["TEXT", "JSON"]),
          }),
        ],
      }),
    ]);
    expect(
      (
        document.getElementById(
          "factory-graph-add-model-operation-input-slot-0-content-type-JSON",
        ) as HTMLInputElement
      ).checked,
    ).toBe(true);
  });
});
