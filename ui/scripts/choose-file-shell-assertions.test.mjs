import { describe, expect, test } from "vitest";
import {
  assertChooseFileDragActiveNeutral,
  assertChooseFileShellNeutral,
} from "./choose-file-shell-assertions.mjs";

describe("choose-file shell assertions", () => {
  test("accepts neutral dashed shell classes", () => {
    expect(() =>
      assertChooseFileShellNeutral(
        "rounded-xl border border-dashed border-outline-variant bg-surface-container-low",
        "shell",
      ),
    ).not.toThrow();
  });

  test("rejects accent-filled shell classes", () => {
    expect(() =>
      assertChooseFileShellNeutral(
        "border-dashed border-outline-variant bg-af-accent-surface",
        "shell",
      ),
    ).toThrow(/bg-af-accent-surface/);
  });

  test("accepts neutral drag-active shell classes", () => {
    expect(() =>
      assertChooseFileDragActiveNeutral(
        "border-dashed border-outline-variant bg-af-overlay",
        "shell",
      ),
    ).not.toThrow();
  });
});
