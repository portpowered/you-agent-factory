import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import { expectStyledCheckbox } from "../../../../../testing/checkbox-test-helpers";

import type { EditableWorkerConfigurationState } from "../../lib/detail-card-types";
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

describe("WorkerEditableConfigurationSection skipPermissions control", () => {
  it("renders the shared styled checkbox for model workers", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

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
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="reviewer"
      />,
    );

    const skipPermissionsCheckbox = screen.getByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    await user.click(screen.getByText(messages.skipPermissionsFieldLabel));
    rerender(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="reviewer"
      />,
    );
    expect(skipPermissionsCheckbox.checked).toBe(true);

    skipPermissionsCheckbox.focus();
    await user.keyboard(" ");
    rerender(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="reviewer"
      />,
    );
    expect(skipPermissionsCheckbox.checked).toBe(false);
  });

  it("exposes invalid state and validation feedback on skipPermissions errors", () => {
    const validationMessage = "skipPermissions must be a boolean.";
    const state = {
      ...buildReadyWorkerEditableConfigurationState(["Review"]),
      canSave: false,
      hasValidationErrors: true,
      validationErrors: {
        skipPermissions: validationMessage,
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={state}
        workerName="reviewer"
      />,
    );

    const skipPermissionsCheckbox = screen.getByRole("checkbox", {
      name: messages.skipPermissionsFieldLabel,
    });

    expectStyledCheckbox(skipPermissionsCheckbox);
    expect(skipPermissionsCheckbox.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByText(validationMessage)).toBeInTheDocument();
  });

  it("does not show the permission bypass toggle for script workers", () => {
    const scriptWorkerState: Extract<
      EditableWorkerConfigurationState,
      { status: "ready" }
    > = {
      ...buildReadyWorkerEditableConfigurationState(["Review"]),
      draft: {
        ...buildReadyWorkerEditableConfigurationState(["Review"]).draft,
        model: "",
        modelProvider: null,
        type: "SCRIPT_WORKER",
        command: "node",
      },
    };

    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={scriptWorkerState}
        workerName="reviewer"
      />,
    );

    expect(
      screen.queryByRole("checkbox", {
        name: messages.skipPermissionsFieldLabel,
      }),
    ).toBeNull();
  });
});
