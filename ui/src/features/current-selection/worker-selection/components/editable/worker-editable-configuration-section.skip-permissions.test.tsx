import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";

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
