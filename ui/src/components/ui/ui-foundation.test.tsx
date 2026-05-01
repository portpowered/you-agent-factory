import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { installDashboardBrowserTestShims } from "../dashboard/test-browser-shims";
import { UIFoundationShowcase } from "./ui-foundation-showcase";

describe("UIFoundationShowcase", () => {
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  it("renders the shared primitive baseline with interactive evidence", async () => {
    const user = userEvent.setup();

    render(<UIFoundationShowcase includeResizable={false} />);

    expect(screen.getByRole("button", { name: "Primary action" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Disabled action" }).hasAttribute("disabled")).toBe(
      true,
    );
    expect(screen.getByRole("textbox", { name: "Showcase request name" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Showcase request text" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Showcase work type" })).toBeTruthy();
    expect(screen.getByRole("img", { name: "Primitive chart showcase" })).toBeTruthy();
    expect(
      screen.getByRole("table", {
        name: "Primitive table foundation for trace and detail surfaces.",
      }),
    ).toBeTruthy();
    expect(screen.getByRole("table", { name: "Primitive data table showcase" })).toBeTruthy();
    expect(screen.getByLabelText("Primitive calendar showcase")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open dialog" }));
    const dialog = await screen.findByRole("dialog", { name: "Export factory" });
    expect(within(dialog).getByRole("button", { name: "Confirm export" })).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    const collapseButton = screen.getByRole("button", { name: "Collapse" });
    await user.click(collapseButton);
    expect(screen.getByRole("button", { name: "Expand" }).getAttribute("aria-expanded")).toBe(
      "false",
    );
  });
});
