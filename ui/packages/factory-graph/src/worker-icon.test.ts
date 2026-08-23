import { describe, expect, it } from "vitest";

import {
  factoryGraphWorkerIconKind,
  factoryGraphWorkerProviderKind,
  factoryGraphWorkerProviderLabel,
  isFactoryGraphKnownWorkerType,
} from "./worker-icon.js";

describe("Factory graph worker compatibility presentation", () => {
  it("uses exact canonical membership for worker kinds", () => {
    expect(isFactoryGraphKnownWorkerType("SCRIPT_WORKER")).toBe(true);
    expect(
      ["script_worker", "SCRIPT_WORKER ", " INFERENCE_WORKER"].map(
        isFactoryGraphKnownWorkerType,
      ),
    ).toEqual([false, false, false]);
  });

  it("does not select a known glyph for unfamiliar worker kinds", () => {
    expect(factoryGraphWorkerIconKind("SCRIPT_WORKER", "CODEX")).toBe("script");
    expect(factoryGraphWorkerIconKind("script_worker", "CODEX")).toBe("worker");
    expect(factoryGraphWorkerIconKind("SCRIPT_WORKER ", "CODEX")).toBe(
      "worker",
    );
    expect(factoryGraphWorkerIconKind("FUTURE_WORKER_KIND", "CLAUDE")).toBe(
      "worker",
    );
    expect(factoryGraphWorkerIconKind(undefined, "CODEX")).toBe("codex");
  });

  it.each([
    ["codex", "codex"],
    [" CODEX ", "codex"],
    ["OpenAI", "codex"],
    ["OpenAI-Codex", "codex"],
    ["Claude", "claude"],
    ["Anthropic", "claude"],
    ["Claude CLI", "claude"],
    ["local_claude", "claude"],
    ["Gemini", "gemini"],
    ["Antigravity", "antigravity"],
    ["AGY", "antigravity"],
  ] as const)("resolves the %s provider alias", (providerId, expectedKind) => {
    expect(factoryGraphWorkerProviderKind(providerId)).toBe(expectedKind);
    expect(factoryGraphWorkerIconKind(undefined, providerId)).toBe(
      expectedKind,
    );
  });

  it.each([undefined, null, "", "future-runner", "openai-codex-extra"])(
    "does not fuzzy-match unsupported provider %s",
    (providerId) => {
      expect(factoryGraphWorkerProviderKind(providerId)).toBeUndefined();
      expect(factoryGraphWorkerIconKind(undefined, providerId)).toBe("worker");
    },
  );

  it("keeps provider labels stable for accessible graph names", () => {
    expect(factoryGraphWorkerProviderLabel("codex")).toBe("Codex/OpenAI");
    expect(factoryGraphWorkerProviderLabel("claude")).toBe("Claude/Anthropic");
    expect(factoryGraphWorkerProviderLabel("gemini")).toBe("Gemini");
    expect(factoryGraphWorkerProviderLabel("antigravity")).toBe("Antigravity");
    expect(factoryGraphWorkerProviderLabel(undefined)).toBeUndefined();
  });
});
