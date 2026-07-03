import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
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

describe("WorkerEditableConfigurationSection shared-impact warnings", () => {
  it("shows worker save-impact warning when multiple workstations reference the worker", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review", "Plan"])}
        workerName="reviewer"
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain(
      "Saving reviewer updates workstations",
    );
    expect(screen.getByRole("alert").textContent).toMatch(/Review.*Plan/);
    expect(
      screen.queryByText(
        messages.editableConfigurationSharedImpactWarningDetail,
      ),
    ).toBeNull();
  });

  it("does not show worker save-impact warning for a single-workstation worker", () => {
    render(
      <WorkerEditableConfigurationSection
        messages={messages}
        state={buildReadyWorkerEditableConfigurationState(["Review"])}
        workerName="reviewer"
      />,
    );

    expect(screen.queryByText(/updates workstations/i)).toBeNull();
  });
});
