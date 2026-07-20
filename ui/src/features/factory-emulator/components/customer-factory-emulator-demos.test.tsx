import "@testing-library/jest-dom/vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import { describe, expect, it, vi } from "vitest";

import { customerFactoryEmulatorDemoFixtures } from "../lib/customer-demo-fixtures";
import { CustomerFactoryEmulatorDemos } from "./customer-factory-emulator-demos";

describe("CustomerFactoryEmulatorDemos", () => {
  it("renders independent ready demos and exposes live and terminal states", async () => {
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

    fireEvent.click(within(success).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(success).getByText(
          "Execute: Preparing the launch summary (1.5 seconds virtual time)",
        ),
      ).toBeVisible(),
    );
    expect(within(failure).getByText("Ready")).toBeVisible();

    fireEvent.click(within(success).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(success).getByRole("region", {
          name: "Successful completion",
        }),
      ).toBeVisible(),
    );
    expect(within(success).getByText("1 completed")).toBeVisible();
    expect(within(failure).queryByText("1 completed")).not.toBeInTheDocument();
  });

  it("falls back to canonical workstation copy during history inspection", async () => {
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

    fireEvent.click(within(demo).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(demo).getByText(/Preparing the launch summary/),
      ).toBeVisible(),
    );
    fireEvent.click(within(demo).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(demo).getByRole("region", { name: "Successful completion" }),
      ).toBeVisible(),
    );

    fireEvent.change(
      within(demo).getByRole("slider", { name: "Select replay tick" }),
      {
        target: { value: "1" },
      },
    );
    await waitFor(() =>
      expect(
        within(demo).getByText(
          "Execute: Working at Execute (1.5 seconds virtual time)",
        ),
      ).toBeVisible(),
    );
    expect(
      within(demo).queryByText(/Preparing the launch summary/),
    ).not.toBeInTheDocument();
    expect(within(demo).getByText("Viewing history")).toBeVisible();
  });
});

describe("CustomerFactoryEmulatorDemos unavailable states", () => {
  it("contains invalid setup to one demo region", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    const invalidScenario = {
      ...customerFactoryEmulatorDemoFixtures.repeatReviewFailure.scenario,
      factory: { name: "wrong-factory" },
    } satisfies FactoryEmulatorScenario;

    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[
          customerFactoryEmulatorDemoFixtures.success,
          {
            ...customerFactoryEmulatorDemoFixtures.repeatReviewFailure,
            scenario: invalidScenario,
          },
        ]}
        locale="en"
      />,
    );

    expect(await screen.findByText("1 Work total")).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "This demo could not be prepared",
    );
    expect(
      screen.getByRole("article", { name: "Straightforward success" }),
    ).toBeVisible();
    consoleError.mockRestore();
  });

  it("presents bundled fixtures without initial Work as unavailable", async () => {
    const fixture = customerFactoryEmulatorDemoFixtures.success;
    const emptyScenario = {
      ...fixture.scenario,
      initialSubmissions: [],
    } satisfies FactoryEmulatorScenario;

    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[{ ...fixture, scenario: emptyScenario }]}
        locale="en"
      />,
    );

    expect(
      await screen.findByText("No initial Work is available for this demo."),
    ).toBeVisible();
    expect(screen.queryByText(/Loading/)).not.toBeInTheDocument();
  });
});
