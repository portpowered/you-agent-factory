import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../../components/dashboard/test-browser-shims";

import type { EditableWorkerConfigurationState } from "../../../lib/detail-card-types";
import {
  buildReadyWorkerEditableConfigurationState,
  workerEditableConfigurationSectionMessages as messages,
} from "../worker-editable-configuration-section.test-helpers";
import { WorkerEditableConfigurationScriptFields } from "./worker-editable-configuration-script-fields";

let restoreBrowserShims: (() => void) | undefined;

const SCRIPT_WORKER_DRAFT = {
  argsText: "check\nlint",
  body: "Run the check",
  command: "make check",
  model: "",
  modelProvider: null,
  type: "SCRIPT_WORKER" as const,
};

function renderScriptFields(
  stateOverrides: Partial<
    Extract<EditableWorkerConfigurationState, { status: "ready" }>
  > = {},
  validationErrors: Record<string, string> = {},
) {
  const state = {
    ...buildReadyWorkerEditableConfigurationState(["Run"]),
    ...stateOverrides,
    draft: {
      ...buildReadyWorkerEditableConfigurationState(["Run"]).draft,
      ...SCRIPT_WORKER_DRAFT,
      ...stateOverrides.draft,
    },
    validationErrors: {
      ...buildReadyWorkerEditableConfigurationState(["Run"]).validationErrors,
      ...validationErrors,
      ...stateOverrides.validationErrors,
    },
  };

  render(
    <WorkerEditableConfigurationScriptFields
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

describe("WorkerEditableConfigurationScriptFields", () => {
  it("renders script field labels and values", () => {
    renderScriptFields();

    expect(
      screen.getByRole("textbox", { name: messages.commandFieldLabel }),
    ).toHaveValue("make check");
    expect(
      screen.getByRole("textbox", { name: messages.argsFieldLabel }),
    ).toHaveValue("check\nlint");
    expect(
      screen.getByRole("textbox", { name: messages.bodyFieldLabel }),
    ).toHaveValue("Run the check");
  });

  it("calls script draft handlers when fields change", () => {
    const state = renderScriptFields();

    fireEvent.change(
      screen.getByRole("textbox", { name: messages.commandFieldLabel }),
      { target: { value: "make test" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: messages.argsFieldLabel }),
      { target: { value: "unit" } },
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: messages.bodyFieldLabel }),
      { target: { value: "Run unit tests" } },
    );

    expect(state.onCommandChange).toHaveBeenCalledWith("make test");
    expect(state.onArgsTextChange).toHaveBeenCalledWith("unit");
    expect(state.onBodyChange).toHaveBeenCalledWith("Run unit tests");
  });

  it("shows validation errors with accessible ids and aria relationships", () => {
    renderScriptFields(
      {},
      {
        args: "Args must be valid JSON lines.",
        body: messages.editableConfigurationScriptCommandOrBodyRequired,
        command: messages.editableConfigurationCommandRequired,
      },
    );

    const commandInput = screen.getByRole("textbox", {
      name: messages.commandFieldLabel,
    });
    const argsInput = screen.getByRole("textbox", {
      name: messages.argsFieldLabel,
    });
    const bodyInput = screen.getByRole("textbox", {
      name: messages.bodyFieldLabel,
    });

    expect(commandInput).toHaveAttribute("aria-invalid", "true");
    expect(commandInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-command-error",
    );
    expect(argsInput).toHaveAttribute("aria-invalid", "true");
    expect(argsInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-args-error",
    );
    expect(bodyInput).toHaveAttribute("aria-invalid", "true");
    expect(bodyInput).toHaveAttribute(
      "aria-describedby",
      "editable-worker-body-error",
    );

    expect(
      screen.getByText(messages.editableConfigurationCommandRequired),
    ).toHaveAttribute("id", "editable-worker-command-error");
    expect(screen.getByText("Args must be valid JSON lines.")).toHaveAttribute(
      "id",
      "editable-worker-args-error",
    );
    expect(
      screen.getByText(
        messages.editableConfigurationScriptCommandOrBodyRequired,
      ),
    ).toHaveAttribute("id", "editable-worker-body-error");
  });

  it("shows server-change hints for overwritten script fields", () => {
    renderScriptFields({
      overwriteFieldNames: ["command", "args", "body"],
    });

    expect(
      screen.getAllByText(messages.editableConfigurationServerFieldChangedHint),
    ).toHaveLength(3);
  });

  it("keeps multiline script fields keyboard reachable", async () => {
    const user = userEvent.setup();
    renderScriptFields();

    const commandInput = screen.getByRole("textbox", {
      name: messages.commandFieldLabel,
    });
    const argsInput = screen.getByRole("textbox", {
      name: messages.argsFieldLabel,
    });
    const bodyInput = screen.getByRole("textbox", {
      name: messages.bodyFieldLabel,
    });

    expect(argsInput.tagName).toBe("TEXTAREA");
    expect(bodyInput.tagName).toBe("TEXTAREA");

    await user.click(commandInput);
    await user.tab();
    expect(argsInput).toHaveFocus();
    await user.tab();
    expect(bodyInput).toHaveFocus();
  });
});
