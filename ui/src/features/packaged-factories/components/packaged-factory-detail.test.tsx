import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import { vi } from "vitest";

import type { PackagedFactoryDetailViewModel } from "../lib/projection";
import { getPackagedFactoryInventoryMessages } from "../messages/inventory";
import { PackagedFactoryDetail } from "./packaged-factory-detail";

const hostile = "<script>do-not-run()</script>";
const detail: PackagedFactoryDetailViewModel = {
  availableFormats: ["json", "yaml"],
  configurations: {
    json: {
      copyValue: `{\n  "value": "${hostile}"\n}`,
      displayValue: `{\n  "value": "${hostile}"\n}`,
      format: "json",
    },
    yaml: {
      copyValue: `value: "${hostile}"\n`,
      displayValue: `value: "${hostile}"\n`,
      format: "yaml",
    },
  },
  description: { status: "unavailable" },
  examples: {
    status: "available",
    items: [
      {
        args: { input: hostile },
        copyValue: `{\n  "factory": "@you/hostile",\n  "args": {\n    "input": "${hostile}"\n  }\n}`,
        description: { status: "available", value: hostile },
        name: hostile,
      },
    ],
  },
  identity: "@you/hostile",
  project: "builtin-hostile",
  slug: "hostile",
  stableName: "@you/hostile",
};

describe("PackagedFactoryDetail", () => {
  it("switches schema-backed formats and copies exactly the visible inert data", async () => {
    const user = userEvent.setup();
    const copyText = vi.fn().mockResolvedValue(undefined);
    const { baseElement } = render(
      <main>
        <PackagedFactoryDetail
          copyText={copyText}
          detail={detail}
          headingID="factory-heading"
          messages={getPackagedFactoryInventoryMessages("en")}
        />
      </main>,
    );

    expect(screen.getByText("Description unavailable")).toBeVisible();
    expect(document.querySelectorAll("code")[0]?.textContent).toBe(
      detail.configurations.json.displayValue,
    );
    await user.click(screen.getByRole("button", { name: "YAML" }));
    expect(document.querySelectorAll("code")[0]?.textContent).toBe(
      detail.configurations.yaml.displayValue,
    );
    expect(
      screen.getByRole("button", { name: "YAML" }).getAttribute("aria-pressed"),
    ).toBe("true");

    await user.click(
      screen.getByRole("button", { name: "Copy YAML configuration" }),
    );
    expect(copyText).toHaveBeenLastCalledWith(
      detail.configurations.yaml.displayValue,
    );
    expect(await screen.findByText("Configuration copied.")).toBeVisible();

    await user.click(
      screen.getByRole("button", {
        name: `Copy ${hostile} invocation example`,
      }),
    );
    if (detail.examples.status !== "available") {
      throw new Error("Expected an invocation example.");
    }
    expect(copyText).toHaveBeenLastCalledWith(
      detail.examples.items[0]?.copyValue,
    );
    expect(document.querySelector("script")).toBeNull();
    expect((await axe(baseElement)).violations).toEqual([]);
  });

  it("reports recoverable copy failure and omits example actions when none exist", async () => {
    const user = userEvent.setup();
    const copyText = vi.fn().mockRejectedValue(new Error("denied"));
    render(
      <PackagedFactoryDetail
        copyText={copyText}
        detail={{ ...detail, examples: { status: "none" } }}
        headingID="factory-heading"
        messages={getPackagedFactoryInventoryMessages("en")}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "Copy JSON configuration" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Could not copy configuration. Try again.",
    );
    expect(
      screen.getByText("No invocation examples are available."),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /invocation example/i }),
    ).toBeNull();
  });
});
