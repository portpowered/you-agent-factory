import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach } from "vitest";

import { App, CLI_COMMAND_EXPLORER_PATH } from "./App";

afterEach(cleanup);

describe("CLI command explorer website route", () => {
  it("renders the published manifest outcome without mounting dashboard content", () => {
    render(
      <App initialLocale="en" locationPathname={CLI_COMMAND_EXPLORER_PATH} />,
    );

    expect(
      screen.getByRole("alert", {
        name: "Unsupported CLI manifest version",
      }),
    ).toHaveTextContent("cli-command-identity/v1");
    expect(
      screen.queryByRole("heading", { name: "Loading dashboard" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("navigation", { name: "Commands" }),
    ).not.toBeInTheDocument();
  });
});
