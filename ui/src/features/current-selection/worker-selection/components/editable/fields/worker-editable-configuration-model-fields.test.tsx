import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../../components/dashboard/test-browser-shims";
import { expectStyledCheckbox } from "../../../../../../testing/checkbox-test-helpers";
import { selectComboboxOption } from "../../../../../../testing/select-test-helpers";

import type { EditableWorkerConfigurationState } from "../../../lib/detail-card-types";
import {
  buildReadyWorkerEditableConfigurationState,
  workerEditableConfigurationSectionMessages as messages,
} from "../worker-editable-configuration-section.test-helpers";
import { WorkerEditableConfigurationModelFields } from "./worker-editable-configuration-model-fields";

let restoreBrowserShims: (() => void) | undefined;

function renderModelFields(
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
    <WorkerEditableConfigurationModelFields
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: model field rendering regressions stay grouped in one field-group harness.
describe("WorkerEditableConfigurationModelFields", () => {
  it("renders model field labels, values, and help text", () => {
    renderModelFields({
      draft: {
        executorProvider: "SCRIPT_WRAP",
        model: "gpt-5.5",
        modelLocality: "LOCAL",
        modelProvider: "CODEX",
        skipPermissions: true,
      },
    });

    expect(
      screen.getByRole("combobox", { name: messages.modelProviderLabel }),
    ).toHaveTextContent(messages.localizeModelProvider("CODEX"));
    expect(
      screen.getByRole("textbox", { name: messages.modelLabel }),
    ).toHaveValue("gpt-5.5");
    expect(
      screen.getByRole("combobox", { name: messages.modelLocalityLabel }),
    ).toHaveTextContent(messages.localizeModelLocality("LOCAL"));
    expect(
      screen.getByRole("combobox", { name: messages.executorProviderLabel }),
    ).toHaveTextContent(messages.localizeExecutorProvider("SCRIPT_WRAP"));
    expect(
      screen.getByRole("checkbox", {
        name: messages.skipPermissionsFieldLabel,
      }),
    ).toBeChecked();
    expect(
      screen.getByText(messages.modelProviderFieldHelp),
    ).toBeInTheDocument();
    expect(screen.getByText(messages.modelFieldHelp)).toBeInTheDocument();
    expect(
      screen.getByText(messages.skipPermissionsFieldHelp),
    ).toBeInTheDocument();
  });

  it("shows the not-configured placeholder for optional model enum selects", () => {
    renderModelFields({
      draft: {
        executorProvider: null,
        modelLocality: null,
        modelProvider: null,
      },
    });

    expect(
      screen.getByRole("combobox", { name: messages.modelProviderLabel }),
    ).toHaveTextContent(messages.notConfiguredOptionLabel);
    expect(
      screen.getByRole("combobox", { name: messages.modelLocalityLabel }),
    ).toHaveTextContent(messages.notConfiguredOptionLabel);
    expect(
      screen.getByRole("combobox", { name: messages.executorProviderLabel }),
    ).toHaveTextContent(messages.notConfiguredOptionLabel);
  });

  it("calls model text draft handlers when fields change", () => {
    const state = renderModelFields();

    fireEvent.change(
      screen.getByRole("textbox", { name: messages.modelLabel }),
      { target: { value: "claude-sonnet" } },
    );

    expect(state.onModelChange).toHaveBeenCalledWith("claude-sonnet");
  });

  it("calls model select draft handlers when combobox values change", async () => {
    const user = userEvent.setup();
    const state = renderModelFields();

    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: messages.modelProviderLabel }),
      messages.localizeModelProvider("CODEX"),
    );
    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: messages.modelLocalityLabel }),
      messages.localizeModelLocality("CLOUD"),
    );
    await selectComboboxOption(
      user,
      screen.getByRole("combobox", { name: messages.executorProviderLabel }),
      messages.localizeExecutorProvider("SCRIPT_WRAP"),
    );

    expect(state.onModelProviderChange).toHaveBeenCalledWith("CODEX");
    expect(state.onModelLocalityChange).toHaveBeenCalledWith("CLOUD");
    expect(state.onExecutorProviderChange).toHaveBeenCalledWith("SCRIPT_WRAP");
  });

  it("shows validation errors with accessible ids and aria relationships", () => {
    renderModelFields(
      {},
      {
        executorProvider: "Invalid executor provider.",
        model: messages.editableConfigurationModelRequired,
        modelLocality: "Invalid model locality.",
        modelProvider: messages.editableConfigurationModelProviderRequired,
      },
    );

    const modelProviderSelect = screen.getByRole("combobox", {
      name: messages.modelProviderLabel,
    });
    const modelInput = screen.getByRole("textbox", {
      name: messages.modelLabel,
    });
    const modelLocalitySelect = screen.getByRole("combobox", {
      name: messages.modelLocalityLabel,
    });
    const executorProviderSelect = screen.getByRole("combobox", {
      name: messages.executorProviderLabel,
    });

    expect(modelProviderSelect).toHaveAttribute("aria-invalid", "true");
    expect(modelProviderSelect).toHaveAttribute(
      "aria-describedby",
      "editable-worker-model-provider-error",
    );
    expect(modelInput).toHaveAttribute("aria-invalid", "true");
    expect(modelInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-model-error",
    );
    expect(modelLocalitySelect).toHaveAttribute("aria-invalid", "true");
    expect(modelLocalitySelect).toHaveAttribute(
      "aria-describedby",
      "editable-worker-model-locality-error",
    );
    expect(executorProviderSelect).toHaveAttribute("aria-invalid", "true");
    expect(executorProviderSelect).toHaveAttribute(
      "aria-describedby",
      "editable-worker-executor-provider-error",
    );

    expect(
      screen.getByText(messages.editableConfigurationModelProviderRequired),
    ).toHaveAttribute("id", "editable-worker-model-provider-error");
    expect(
      screen.getByText(messages.editableConfigurationModelRequired),
    ).toHaveAttribute("id", "editable-worker-model-error");
    expect(screen.getByText("Invalid model locality.")).toHaveAttribute(
      "id",
      "editable-worker-model-locality-error",
    );
    expect(screen.getByText("Invalid executor provider.")).toHaveAttribute(
      "id",
      "editable-worker-executor-provider-error",
    );
  });

  it("shows server-change hints for overwritten model fields", () => {
    renderModelFields({
      overwriteFieldNames: [
        "modelProvider",
        "model",
        "modelLocality",
        "executorProvider",
        "skipPermissions",
      ],
    });

    expect(
      screen.getAllByText(messages.editableConfigurationServerFieldChangedHint),
    ).toHaveLength(5);
  });

  it("renders the shared styled checkbox for skipPermissions", () => {
    renderModelFields();

    const skipPermissionsCheckbox = screen.getByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    expectStyledCheckbox(skipPermissionsCheckbox);
    expect(skipPermissionsCheckbox.checked).toBe(false);
    expect(
      screen.getByText(messages.skipPermissionsFieldHelp),
    ).toBeInTheDocument();
  });

  it("toggles skipPermissions from label clicks and Space while focused", async () => {
    const user = userEvent.setup();
    const state = buildReadyWorkerEditableConfigurationState(["Review"]);
    state.onSkipPermissionsChange = vi.fn((checked: boolean) => {
      state.draft.skipPermissions = checked;
    });

    const { rerender } = render(
      <WorkerEditableConfigurationModelFields
        messages={messages}
        state={state}
        validationErrors={state.validationErrors}
      />,
    );

    const skipPermissionsCheckbox = screen.getByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    await user.click(screen.getByText(messages.skipPermissionsFieldLabel));
    rerender(
      <WorkerEditableConfigurationModelFields
        messages={messages}
        state={state}
        validationErrors={state.validationErrors}
      />,
    );
    expect(skipPermissionsCheckbox.checked).toBe(true);

    skipPermissionsCheckbox.focus();
    await user.keyboard(" ");
    rerender(
      <WorkerEditableConfigurationModelFields
        messages={messages}
        state={state}
        validationErrors={state.validationErrors}
      />,
    );
    expect(skipPermissionsCheckbox.checked).toBe(false);
  });

  it("exposes invalid state and validation feedback on skipPermissions errors", () => {
    const validationMessage = "skipPermissions must be a boolean.";

    renderModelFields(
      {},
      {
        skipPermissions: validationMessage,
      },
    );

    const skipPermissionsCheckbox = screen.getByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    expectStyledCheckbox(skipPermissionsCheckbox);
    expect(skipPermissionsCheckbox.getAttribute("aria-invalid")).toBe("true");
    expect(skipPermissionsCheckbox).toHaveAttribute(
      "aria-describedby",
      "editable-worker-skip-permissions-error",
    );
    expect(screen.getByText(validationMessage)).toHaveAttribute(
      "id",
      "editable-worker-skip-permissions-error",
    );
  });
});
