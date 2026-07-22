import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach } from "vitest";

import canonicalCliManifest from "../../../../../contracts/cli/commands.json" with {
  type: "json",
};
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import {
  loadCliManifest,
  loadingCliManifest,
} from "../lib/cli-manifest-adapter";
import type { CliManifestLoadState } from "../lib/cli-manifest-types";
import { StaticCliCommandExplorer } from "./static-cli-command-explorer";

function readyState(): CliManifestLoadState {
  return loadCliManifest(canonicalCliManifest);
}

describe("static CLI command explorer", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it.each([
    [loadingCliManifest(), "Loading CLI commands", "status"],
    [
      {
        status: "empty",
        manifest: { formatVersion: "1.0.0", rootPath: "you", commands: {} },
      },
      "No commands available",
      "status",
    ],
    [
      {
        status: "unsupported-version",
        receivedVersion: "2.0.0",
        supportedVersions: ["1.0.0"],
      },
      "Unsupported CLI manifest version",
      "alert",
    ],
    [
      {
        status: "invalid-contract",
        diagnostics: [
          {
            code: "missing_field",
            path: ["rootPath"],
            message: "Expected required field rootPath.",
          },
        ],
      },
      "Invalid CLI contract",
      "alert",
    ],
  ] satisfies readonly [CliManifestLoadState, string, string][])(
    "renders the %s outcome without partial command detail",
    (state, title, role) => {
      render(<StaticCliCommandExplorer state={state} />);
      expect(screen.getByRole(role, { name: title })).toBeVisible();
      expect(
        screen.queryByRole("navigation", { name: "Commands" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("heading", { name: "Usage" }),
      ).not.toBeInTheDocument();
    },
  );

  it("selects root and nested commands through accessible tree activation", async () => {
    const user = userEvent.setup();
    render(<StaticCliCommandExplorer state={readyState()} />);

    const navigation = screen.getByRole("navigation", { name: "Commands" });
    const root = screen.getByRole("treeitem", {
      name: /^youLifecycle: active$/,
    });
    expect(root).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Run and manage CPN-based workflow factories",
    );

    const run = screen.getByRole("treeitem", {
      name: /^you runLifecycle: active$/,
    });
    await user.click(run);
    expect(run).toHaveAttribute("aria-selected", "true");
    expect(navigation).toHaveTextContent("you run");
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(
      "Load workflow and run the factory engine",
    );
    expect(
      screen.getAllByText("you run --work ./docs/examples/startup-work.json", {
        exact: false,
      })[0],
    ).toBeVisible();
    expect(
      screen.getByText("mutually-exclusive: --dir, --named, --factory"),
    ).toBeVisible();
    expect(
      screen.getByRole("checkbox", { name: "--verbose" }),
    ).toHaveAccessibleDescription(
      expect.stringContaining("Inherited global input"),
    );
  });

  it("supports roving keyboard traversal and explicit activation", async () => {
    const user = userEvent.setup();
    render(<StaticCliCommandExplorer state={readyState()} />);
    const root = screen.getByRole("treeitem", {
      name: /^youLifecycle: active$/,
    });
    root.focus();
    fireEvent.keyDown(root, { key: "ArrowDown" });
    const next = screen.getAllByRole("treeitem")[1];
    expect(next).toHaveFocus();
    expect(root).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{Enter}");
    expect(next).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("heading", { level: 1 })).not.toHaveTextContent(
      "Run and manage CPN-based workflow factories",
    );
  });
});

describe("localized static CLI command explorer", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("localizes the selected nested command surface", async () => {
    const user = userEvent.setup();
    render(<StaticCliCommandExplorer locale="zh-CN" state={readyState()} />);

    expect(screen.getByRole("navigation", { name: "命令" })).toBeVisible();
    await user.click(
      screen.getByRole("treeitem", {
        name: /^you run生命周期：active$/,
      }),
    );

    expect(
      screen.getByRole("region", { name: "已选择命令：you run" }),
    ).toBeVisible();
    expect(screen.getByRole("heading", { name: "用法" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "示例" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "输入关系" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "静态输入" })).toBeVisible();
    expect(
      screen.getByText("mutually-exclusive：--dir, --named, --factory"),
    ).toBeVisible();
  });
});
