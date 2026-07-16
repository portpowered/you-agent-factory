import { describe, expect, test, vi } from "vitest";
import {
  expectNoHorizontalOverflow,
  expectVisible,
} from "./storybook-responsive-helpers.mjs";
import { verifyProviderSessionDetailSuccess } from "./verify-provider-session-storybook-responsive.mjs";

function createDisclosureLocator(state, stateKey) {
  return {
    click: vi.fn().mockImplementation(async () => {
      state[stateKey] = !state[stateKey];
    }),
    getAttribute: vi
      .fn()
      .mockImplementation(async () => String(state[stateKey])),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };
}

function createSelectedSessionHeading() {
  const preview = {
    dd: [
      "019e44f4-580e-7f32-981e-1e54ec6907d6",
      "32",
      "18",
      "0",
      "2026/05/20/rollout-2026-05-20T17-35-24-019e44f4-580e-7f32-981e-1e54ec6907d6.jsonl",
    ],
    dt: [
      "Session ID",
      "Input Tokens",
      "Output Tokens",
      "Cached Tokens",
      "Source File",
    ],
  };
  return {
    locator: vi.fn().mockReturnValue({
      locator: vi.fn((selector) => ({
        allTextContents: vi.fn().mockResolvedValue(preview[selector]),
      })),
    }),
    waitFor: vi.fn().mockResolvedValue(undefined),
  };
}

function createProviderSessionPage(state) {
  const visibleLocator = () => ({
    waitFor: vi.fn().mockResolvedValue(undefined),
  });
  const selectedSessionHeading = createSelectedSessionHeading();
  const entryToggles = {
    count: vi.fn().mockResolvedValue(6),
    nth: vi.fn().mockReturnValue({
      getAttribute: vi.fn().mockResolvedValue("true"),
    }),
  };
  return {
    evaluate: vi.fn().mockResolvedValue({ clientWidth: 390, scrollWidth: 390 }),
    getByRole: vi.fn((role, options) => {
      if (role === "heading") {
        if (!options) {
          return {
            allTextContents: vi
              .fn()
              .mockResolvedValue([
                "Selected Session Details",
                "Source Metadata",
                "Session Analysis",
                "Transcript",
              ]),
          };
        }
        if (options.name === "Selected session details") {
          return selectedSessionHeading;
        }
        if (
          options.name === "Source file" ||
          options.name === "Session analysis"
        ) {
          return { count: vi.fn().mockResolvedValue(0) };
        }
        return visibleLocator();
      }
      if (role === "button" && options.name instanceof RegExp) {
        return entryToggles;
      }
      if (role === "button") {
        const stateKey = options.name.includes("Transcript")
          ? "transcript"
          : options.name.includes("User")
            ? "userMessage"
            : "selectedSession";
        return createDisclosureLocator(state, stateKey);
      }
      throw new Error(`unexpected role lookup ${role}`);
    }),
    getByText: vi.fn().mockReturnValue(visibleLocator()),
    waitForSelector: vi.fn().mockResolvedValue(undefined),
  };
}

describe("provider-session story assertions", () => {
  test("checks the consolidated default-open experience", async () => {
    const state = {
      selectedSession: false,
      transcript: true,
      userMessage: true,
    };
    const page = createProviderSessionPage(state);

    await verifyProviderSessionDetailSuccess({
      expectNoHorizontalOverflow,
      expectVisible,
      page,
      viewport: { height: 844, label: "mobile", width: 390 },
    });

    expect(state).toEqual({
      selectedSession: true,
      transcript: true,
      userMessage: true,
    });
    expect(page.getByRole).toHaveBeenCalledWith("button", {
      name: "Expand Selected Session Details",
    });
  });
});
