import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";

import { WorkerEditableConfigurationSection } from "./worker-editable-configuration-section";
import {
  buildReadyWorkerEditableConfigurationState,
  workerEditableConfigurationSectionMessages as messages,
} from "./worker-editable-configuration-section.test-helpers";

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("WorkerEditableConfigurationSection shell states", () => {
  it("shows loading feedback while editable configuration is loading", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{ status: "loading" }}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText(messages.editableConfigurationLoading),
    ).toBeInTheDocument();
  });

  it("shows error feedback with alert role when configuration is unavailable", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{ errorMessage: "network timeout", status: "error" }}
        workerName="reviewer"
      />,
    );

    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain(
      messages.editableConfigurationErrorPrefix,
    );
    expect(alert.textContent).toContain("network timeout");
  });

  it("shows empty feedback with the default empty message", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{ status: "empty" }}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText(messages.editableConfigurationEmpty),
    ).toBeInTheDocument();
  });

  it("shows a custom empty message when provided", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{ message: "No editable worker selected.", status: "empty" }}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText("No editable worker selected."),
    ).toBeInTheDocument();
  });

  it("expands and collapses the editable configuration section", async () => {
    const user = userEvent.setup();

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    const toggle = screen.getByRole("button", {
      name: messages.editableConfigurationCollapseActionLabel,
    });
    expect(
      screen.getByRole("textbox", { name: messages.nameFieldLabel }),
    ).toBeInTheDocument();

    await user.click(toggle);

    expect(
      screen.getByRole("button", {
        name: messages.editableConfigurationExpandActionLabel,
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("textbox", { name: messages.nameFieldLabel }),
    ).toBeNull();
  });
});

describe("WorkerEditableConfigurationSection ready-state feedback", () => {
  it("shows validation status and save-disabled detail when draft validation fails", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{
          ...buildReadyWorkerEditableConfigurationState(["Review"]),
          canSave: false,
          hasValidationErrors: true,
          validationErrors: {
            name: messages.editableConfigurationNameRequired,
          },
        }}
        workerName="reviewer"
      />,
    );

    expect(
      screen.getByText(messages.editableConfigurationValidationStatus),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        messages.editableConfigurationSaveDisabledValidationDetail,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(messages.editableConfigurationNameRequired),
    ).toBeInTheDocument();
  });

  it("shows contract-level errors from draft validation", () => {
    const contractMessage = `${messages.editableConfigurationContractInvalidPrefix} Invalid worker type.`;

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{
          ...buildReadyWorkerEditableConfigurationState(["Review"]),
          validationErrors: {
            contract: contractMessage,
          },
        }}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(contractMessage);
  });

  it("shows overwrite warning when server-side fields changed", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={{
          ...buildReadyWorkerEditableConfigurationState(["Review"]),
          overwriteFieldNames: ["name", "model"],
        }}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "The running factory changed after you started editing",
    );
  });

  it("merges save-state field errors with draft validation errors", () => {
    const saveNameError =
      messages.editableConfigurationNameDuplicate("other-worker");

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        saveState={{
          errorMessage: "Save validation failed.",
          fieldErrors: {
            name: saveNameError,
          },
          status: "error",
        }}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(screen.getByText(saveNameError)).toBeInTheDocument();
  });
});
