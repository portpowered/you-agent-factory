import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach } from "vitest";

import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { projectCliManifest } from "../lib/cli-command-projection";
import {
  type CliControlModel,
  projectCliCommandControls,
} from "../lib/cli-control-projection";
import { loadCliManifest } from "../lib/cli-manifest-adapter";
import { StaticCliControls } from "./static-cli-controls";

function commandControls(commandId: string): CliControlModel {
  const loaded = loadCliManifest(canonicalCliManifest);
  if (loaded.status !== "ready") throw new Error("Expected ready manifest.");
  const projected = projectCliCommandControls(
    projectCliManifest(loaded).commands[commandId],
  );
  if (projected.status !== "ready") throw new Error("Expected controls.");
  return projected.model;
}

describe("static CLI controls", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders accessible local, inherited, required, repeated, and choice controls", async () => {
    const user = userEvent.setup();
    const docs = commandControls("you.docs");
    render(<StaticCliControls model={docs} />);

    const topic = screen.getByRole("combobox", { name: "<topic>" });
    await user.click(topic);
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "agents",
      }),
    );
    expect(topic).toHaveTextContent("agents");
    expect(
      screen.getByRole("checkbox", { name: "--verbose" }),
    ).toHaveAccessibleDescription(
      expect.stringContaining("Inherited global input"),
    );

    cleanup();
    const create = commandControls("you.factory.create");
    render(<StaticCliControls model={create} />);
    const requiredName = screen.getByRole("textbox", { name: "<name>" });
    expect(requiredName).toBeRequired();
    expect(requiredName).toHaveAttribute("aria-invalid", "true");
    expect(requiredName).toHaveAccessibleErrorMessage(
      expect.stringContaining("requires between 1 and 1 values"),
    );

    cleanup();
    const run = commandControls("you.run");
    render(<StaticCliControls model={run} />);
    const repeated = screen.getByRole("textbox", {
      name: "<invocation-input> value 1",
    });
    await user.type(repeated, "first");
    await user.click(
      screen.getByRole("button", {
        name: "Add another <invocation-input> value",
      }),
    );
    expect(
      screen.getByRole("textbox", { name: "<invocation-input> value 2" }),
    ).toBeVisible();
  });

  it("associates relationship guidance and deterministic errors with every affected field", async () => {
    const user = userEvent.setup();
    const run = commandControls("you.run");
    render(<StaticCliControls model={run} />);

    const directory = screen.getByRole("textbox", { name: "--dir" });
    const named = screen.getByRole("textbox", { name: "--named" });
    expect(directory).toHaveAccessibleDescription(
      expect.stringContaining("Related inputs: --named, --factory"),
    );
    await user.type(named, "example");

    expect(directory).not.toHaveAttribute("aria-invalid", "true");
    expect(named).not.toHaveAttribute("aria-invalid", "true");

    await user.clear(directory);
    await user.type(directory, "./factory");

    expect(directory).toHaveAttribute("aria-invalid", "true");
    expect(named).toHaveAttribute("aria-invalid", "true");
    expect(directory).toHaveAccessibleErrorMessage(
      expect.stringContaining("conflicts with --named, --factory"),
    );
  });

  it("does not render manifest inputs hidden from default help", () => {
    const run = commandControls("you.run");
    render(<StaticCliControls model={run} />);

    expect(
      screen.queryByRole("textbox", { name: "--port" }),
    ).not.toBeInTheDocument();
  });

  it("localizes repeated controls and relationship feedback", async () => {
    const user = userEvent.setup();
    const run = commandControls("you.run");
    render(<StaticCliControls locale="zh-CN" model={run} />);

    const directory = screen.getByRole("textbox", { name: "--dir" });
    expect(directory).toHaveAccessibleDescription(
      expect.stringContaining("相关输入：--named, --factory。"),
    );
    expect(
      screen.getByRole("textbox", { name: "<invocation-input> 的第 1 个值" }),
    ).toBeVisible();
    await user.click(
      screen.getByRole("button", {
        name: "添加另一个 <invocation-input> 值",
      }),
    );
    expect(
      screen.getByRole("button", {
        name: "移除 <invocation-input> 的第 1 个值",
      }),
    ).toBeVisible();

    await user.type(directory, "./factory");
    await user.type(
      screen.getByRole("textbox", { name: "--named" }),
      "example",
    );
    expect(directory).toHaveAccessibleErrorMessage(
      expect.stringContaining("此值与 --named, --factory 冲突。"),
    );
  });
});

describe("static CLI dependency controls", () => {
  afterEach(cleanup);

  it("renders localized dependency guidance and errors", async () => {
    const user = userEvent.setup();
    const dependencyModel = {
      commandId: "you.example",
      controls: [
        {
          id: "cli-control-trigger",
          inputId: "you.example.flag.trigger",
          label: "--trigger",
          required: false,
          inherited: false,
          defaultValue: false,
          cardinality: { minimum: 0, maximum: 1 },
          relationshipIds: ["you.example.rel.dependency"],
          kind: "boolean",
        },
        {
          id: "cli-control-target",
          inputId: "you.example.flag.target",
          label: "--target",
          required: false,
          inherited: false,
          defaultValue: "",
          cardinality: { minimum: 0, maximum: 1 },
          relationshipIds: ["you.example.rel.dependency"],
          kind: "text",
        },
      ],
      relationships: [
        {
          id: "you.example.rel.dependency",
          kind: "dependency",
          participants: [{ inputId: "you.example.flag.target" }],
          when: { inputId: "you.example.flag.trigger" },
        },
      ],
    } as unknown as CliControlModel;
    render(<StaticCliControls locale="zh-CN" model={dependencyModel} />);

    const trigger = screen.getByRole("checkbox", { name: "--trigger" });
    const target = screen.getByRole("textbox", { name: "--target" });
    expect(target).toHaveAccessibleDescription(
      expect.stringContaining("相关输入：--trigger。"),
    );
    await user.click(trigger);
    expect(target).toHaveAccessibleErrorMessage(
      "提供 --trigger 时必须填写此值。",
    );
  });
});
