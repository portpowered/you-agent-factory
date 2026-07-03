import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../../components/dashboard/test-browser-shims";
import { selectComboboxOption } from "../../../../../../testing/select-test-helpers";

import type { EditableWorkerConfigurationState } from "../../../lib/detail-card-types";
import {
  buildReadyWorkerEditableConfigurationState,
  workerEditableConfigurationSectionMessages as messages,
} from "../worker-editable-configuration-section.test-helpers";
import { WorkerEditableConfigurationSharedFields } from "./worker-editable-configuration-shared-fields";

let restoreBrowserShims: (() => void) | undefined;

function renderSharedFields(
  stateOverrides: Partial<
    Extract<EditableWorkerConfigurationState, { status: "ready" }>
  > = {},
  validationErrors: Record<string, string> = {},
) {
  const state = {
    ...buildReadyWorkerEditableConfigurationState(["Review"]),
    ...stateOverrides,
    draft: {
      ...buildReadyWorkerEditableConfigurationState(["Review"]).draft,
      ...stateOverrides.draft,
    },
    validationErrors: {
      ...buildReadyWorkerEditableConfigurationState(["Review"])
        .validationErrors,
      ...validationErrors,
      ...stateOverrides.validationErrors,
    },
  };

  render(
    <WorkerEditableConfigurationSharedFields
      messages={messages}
      state={state}
      validationErrors={state.validationErrors}
    />,
  );

  return state;
}

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: shared field rendering regressions stay grouped in one field-group harness.
describe("WorkerEditableConfigurationSharedFields", () => {
  it("renders shared field labels, values, and help text", () => {
    renderSharedFields({
      draft: {
        name: "reviewer",
        stopToken: "STOP",
        timeoutAmount: "30",
        timeoutUnit: "s",
        type: "MODEL_WORKER",
      },
    });

    expect(
      screen.getByRole("textbox", { name: messages.nameFieldLabel }),
    ).toHaveValue("reviewer");
    expect(
      screen.getByRole("combobox", { name: messages.typeFieldLabel }),
    ).toHaveTextContent(messages.localizeWorkerType("MODEL_WORKER"));
    expect(
      screen.getByRole("spinbutton", { name: messages.timeoutFieldLabel }),
    ).toHaveValue(30);
    expect(
      screen.getByRole("combobox", { name: messages.timeoutFieldLabel }),
    ).toHaveTextContent(messages.localizeTimeoutUnit("s"));
    expect(
      screen.getByRole("textbox", { name: messages.stopTokenFieldLabel }),
    ).toHaveValue("STOP");
    expect(screen.getByText(messages.timeoutFieldHelp)).toBeInTheDocument();
    expect(screen.getByText(messages.stopTokenFieldHelp)).toBeInTheDocument();
  });

  it("shows the timeout picker for script workers", () => {
    renderSharedFields({
      draft: {
        command: "node",
        model: "",
        modelProvider: null,
        type: "SCRIPT_WORKER",
      },
    });

    expect(
      screen.getByRole("spinbutton", { name: messages.timeoutFieldLabel }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("combobox", { name: messages.timeoutFieldLabel }),
    ).toBeInTheDocument();
  });

  it("shows the stop token input for script workers", () => {
    renderSharedFields({
      draft: {
        command: "node",
        model: "",
        modelProvider: null,
        type: "SCRIPT_WORKER",
      },
    });

    expect(
      screen.getByRole("textbox", { name: messages.stopTokenFieldLabel }),
    ).toBeInTheDocument();
  });

  it("uses the not-configured placeholder and disables the timeout unit when amount is empty", () => {
    renderSharedFields({
      draft: {
        timeoutAmount: "",
        timeoutUnit: "m",
      },
    });

    const amountInput = screen.getByRole("spinbutton", {
      name: messages.timeoutFieldLabel,
    });
    const unitSelect = screen.getByRole("combobox", {
      name: messages.timeoutFieldLabel,
    });

    expect(amountInput).toHaveAttribute(
      "placeholder",
      messages.notConfiguredOptionLabel,
    );
    expect(amountInput).toHaveValue(null);
    expect(unitSelect).toBeDisabled();
  });

  it("calls shared text draft handlers when fields change", () => {
    const state = renderSharedFields();

    fireEvent.change(
      screen.getByRole("textbox", { name: messages.nameFieldLabel }),
      { target: { value: "writer" } },
    );
    fireEvent.change(
      screen.getByRole("spinbutton", { name: messages.timeoutFieldLabel }),
      { target: { value: "45" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: messages.stopTokenFieldLabel }),
      { target: { value: "HALT" } },
    );

    expect(state.onNameChange).toHaveBeenCalledWith("writer");
    expect(state.onTimeoutAmountChange).toHaveBeenCalledWith("45");
    expect(state.onStopTokenChange).toHaveBeenCalledWith("HALT");
  });

  it("calls shared select draft handlers when combobox values change", async () => {
    const user = userEvent.setup();
    const state = renderSharedFields({
      draft: {
        timeoutAmount: "30",
        timeoutUnit: "m",
      },
    });

    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: messages.typeFieldLabel }),
      messages.localizeWorkerType("SCRIPT_WORKER"),
    );
    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: messages.timeoutFieldLabel }),
      messages.localizeTimeoutUnit("h"),
    );

    expect(state.onTypeChange).toHaveBeenCalledWith("SCRIPT_WORKER");
    expect(state.onTimeoutUnitChange).toHaveBeenCalledWith("h");
  });

  it("shows validation errors with accessible ids and aria relationships", () => {
    renderSharedFields(
      {},
      {
        name: messages.editableConfigurationNameRequired,
        stopToken: "Invalid stop token.",
        timeout: messages.editableConfigurationTimeoutInvalid("0"),
        type: "Invalid worker type.",
      },
    );

    const nameInput = screen.getByRole("textbox", {
      name: messages.nameFieldLabel,
    });
    const typeSelect = screen.getByRole("combobox", {
      name: messages.typeFieldLabel,
    });
    const timeoutAmountInput = screen.getByRole("spinbutton", {
      name: messages.timeoutFieldLabel,
    });
    const stopTokenInput = screen.getByRole("textbox", {
      name: messages.stopTokenFieldLabel,
    });

    expect(nameInput).toHaveAttribute("aria-invalid", "true");
    expect(nameInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-name-error",
    );
    expect(typeSelect).toHaveAttribute("aria-invalid", "true");
    expect(typeSelect).toHaveAttribute(
      "aria-describedby",
      "editable-worker-type-error",
    );
    expect(timeoutAmountInput).toHaveAttribute("aria-invalid", "true");
    expect(timeoutAmountInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-timeout-amount-error",
    );
    expect(stopTokenInput).toHaveAttribute("aria-invalid", "true");
    expect(stopTokenInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-stop-token-error",
    );

    expect(
      screen.getByText(messages.editableConfigurationNameRequired),
    ).toHaveAttribute("id", "editable-worker-name-error");
    expect(screen.getByText("Invalid worker type.")).toHaveAttribute(
      "id",
      "editable-worker-type-error",
    );
    expect(
      screen.getByText(messages.editableConfigurationTimeoutInvalid("0")),
    ).toHaveAttribute("id", "editable-worker-timeout-amount-error");
    expect(screen.getByText("Invalid stop token.")).toHaveAttribute(
      "id",
      "editable-worker-stop-token-error",
    );
  });

  it("uses help text ids for aria-describedby when shared fields are valid", () => {
    renderSharedFields();

    expect(
      screen.getByRole("spinbutton", { name: messages.timeoutFieldLabel }),
    ).toHaveAttribute("aria-describedby", "editable-worker-timeout-hint");
    expect(
      screen.getByRole("textbox", { name: messages.stopTokenFieldLabel }),
    ).toHaveAttribute("aria-describedby", "editable-worker-stop-token-hint");
  });

  it("shows server-change hints for overwritten shared fields", () => {
    renderSharedFields({
      overwriteFieldNames: ["name", "type", "timeout", "stopToken"],
    });

    expect(
      screen.getAllByText(messages.editableConfigurationServerFieldChangedHint),
    ).toHaveLength(4);
  });
});
