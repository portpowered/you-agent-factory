import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach } from "vitest";

import { App, CLI_COMMAND_EXPLORER_PATH } from "./App";

afterEach(cleanup);

describe("CLI command explorer website route", () => {
  it("renders root and nested navigation from the published manifest at the deployed path", async () => {
    const user = userEvent.setup();
    render(
      <App initialLocale="en" locationPathname={CLI_COMMAND_EXPLORER_PATH} />,
    );

    expect(screen.getByRole("navigation", { name: "Commands" })).toBeVisible();
    expect(
      screen.getByRole("treeitem", { name: /^youLifecycle: active$/ }),
    ).toHaveAttribute("aria-selected", "true");

    const nestedCommand = screen.getByRole("treeitem", {
      name: /^you runLifecycle: active$/,
    });
    await user.click(nestedCommand);
    expect(nestedCommand).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Load workflow and run the factory engine",
    );
    expect(
      screen.queryByRole("heading", { name: "Loading dashboard" }),
    ).not.toBeInTheDocument();
  });
});
