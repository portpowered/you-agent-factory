import "@testing-library/jest-dom/vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { customerFactoryEmulatorDemoFixtures } from "../lib/customer-demo-fixtures";
import { CustomerFactoryEmulatorDemos } from "./customer-factory-emulator-demos";

function installReducedMotionEnvironment() {
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      public disconnect() {}
      public observe() {}
      public unobserve() {}
    },
  );
  vi.stubGlobal("matchMedia", () => ({
    addEventListener: () => undefined,
    addListener: () => undefined,
    dispatchEvent: () => true,
    matches: true,
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    removeEventListener: () => undefined,
    removeListener: () => undefined,
  }));
}

afterEach(() => vi.unstubAllGlobals());

describe("CustomerFactoryEmulatorDemos text submission", () => {
  it("submits multiline Work during execution without changing the sibling demo", async () => {
    const user = userEvent.setup();
    installReducedMotionEnvironment();
    render(<CustomerFactoryEmulatorDemos locale="en" />);
    const success = screen.getByRole("article", {
      name: "Straightforward success",
    });
    const failure = screen.getByRole("article", {
      name: "Review, rework, and failure",
    });
    await waitFor(() =>
      expect(within(success).getByText("1 Work total")).toBeVisible(),
    );
    expect(within(failure).getByText("1 Work total")).toBeVisible();

    await user.click(within(success).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(success).getByText(/Preparing the launch summary/),
      ).toBeVisible(),
    );
    const textarea = within(success).getByRole("textbox", {
      name: "Submit text",
    });
    await user.type(
      textarea,
      "Customer request{Shift>}{Enter}{/Shift}Second line",
    );
    expect(textarea).toHaveValue("Customer request\nSecond line");
    await user.type(textarea, "{Enter}");

    await waitFor(() => expect(textarea).toHaveValue(""));
    expect(textarea).toHaveFocus();
    expect(within(success).getByText("2 Work total")).toBeVisible();
    expect(within(failure).getByText("1 Work total")).toBeVisible();
  });

  it("disables submission in history and after closure, then restores it at the live head", async () => {
    const user = userEvent.setup();
    installReducedMotionEnvironment();
    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    const demo = screen.getByRole("article", {
      name: "Straightforward success",
    });
    await waitFor(() =>
      expect(within(demo).getByText("1 Work total")).toBeVisible(),
    );
    const textarea = within(demo).getByRole("textbox", {
      name: "Submit text",
    });

    await user.click(within(demo).getByRole("button", { name: "Step" }));
    fireEvent.change(
      within(demo).getByRole("slider", { name: "Select replay tick" }),
      { target: { value: "0" } },
    );
    expect(textarea).toBeDisabled();
    expect(
      within(demo).getByText("Return to the current tick before submitting."),
    ).toBeVisible();

    await user.click(
      within(demo).getByRole("button", { name: "Follow current" }),
    );
    expect(textarea).toBeEnabled();
    await user.click(within(demo).getByRole("button", { name: "Step" }));
    await within(demo).findByRole("region", {
      name: "Successful completion",
    });
    await waitFor(() => expect(textarea).toBeDisabled());
    expect(
      within(demo).getByText(
        "Restart the completed emulator to submit more Work.",
      ),
    ).toBeVisible();
  });
});
