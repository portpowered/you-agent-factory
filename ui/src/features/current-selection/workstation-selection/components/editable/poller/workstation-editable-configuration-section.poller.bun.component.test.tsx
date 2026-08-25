import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { cleanup, render, screen } from "@testing-library/react";

import { installDashboardBrowserTestShims } from "../../../../../../components/dashboard/test-browser-shims";
import { EditableConfigurationSection } from "../workstation-editable-configuration-section";
import {
  buildEditableConfigurationSectionReadyState,
  editableConfigurationSectionMessages,
  expandEditableConfigurationSection,
} from "../workstation-editable-configuration-section.test-helpers";

const messages = editableConfigurationSectionMessages;

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("EditableConfigurationSection poller workstation fields", () => {
  it("keeps poller behavior fixed, filters workers, and omits prompt and runner fields", () => {
    render(
      <EditableConfigurationSection
        messages={messages}
        state={buildEditableConfigurationSectionReadyState({
          workstationType: "POLLER_RUN",
        })}
      />,
    );

    expandEditableConfigurationSection();

    expect(screen.getByRole("combobox", { name: "Worker" })).toHaveTextContent(
      "linear-poller",
    );
    expect(screen.getByRole("combobox", { name: "Kind" })).toHaveTextContent(
      "Poller",
    );
    expect(screen.queryByLabelText("Runner")).toBeNull();
    expect(screen.queryByLabelText("Prompt")).toBeNull();
    expect(
      screen.queryByRole("combobox", { name: "Workstation type" }),
    ).toBeNull();
  });
});
