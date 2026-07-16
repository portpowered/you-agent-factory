import { fireEvent, render, screen } from "@testing-library/react";

import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { TranscriptSection } from "./provider-session-transcript";

describe("TranscriptSection", () => {
  it("opens assistant transcript text by default and remains collapsible", () => {
    const longAssistantText = `${"assistant transcript detail ".repeat(18)}final-visible-marker`;

    render(
      <TranscriptSection
        detail={buildProviderSessionDetailResponse({
          transcript: [
            {
              order: 1,
              text: longAssistantText,
              type: "assistant_message",
            },
          ],
        })}
      />,
    );

    expect(screen.getByText(/final-visible-marker/)).toBeTruthy();
    const toggle = screen.getByRole("button", { name: "Collapse Assistant" });
    expect(toggle.getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText(/final-visible-marker/)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Expand Assistant" }));
    expect(screen.getByText(/final-visible-marker/)).toBeTruthy();
  });

  it("formats encrypted reasoning payloads as transcript code content", () => {
    render(
      <TranscriptSection
        detail={buildProviderSessionDetailResponse({
          transcript: [
            {
              encrypted: true,
              encryptedContent: "sealed-chatgpt-reasoning-blob",
              order: 1,
              sourceType: "reasoning",
              type: "reasoning",
            },
          ],
        })}
      />,
    );

    expect(screen.getAllByText("Encrypted Reasoning").length).toBeGreaterThan(
      0,
    );
    expect(
      screen
        .getAllByText("Encrypted Reasoning")
        .some((element) => element.className.includes("bg-info-container")),
    ).toBe(true);
    expect(screen.getByText("sealed-chatgpt-reasoning-blob")).toBeTruthy();
    expect(screen.queryByText("Encrypted reasoning content only.")).toBeNull();
  });

  it("renders transcript entry statuses through compact status pills", () => {
    render(
      <TranscriptSection
        detail={buildProviderSessionDetailResponse({
          transcript: [
            {
              order: 1,
              status: "completed",
              text: "Done",
              type: "assistant_message",
            },
          ],
        })}
      />,
    );

    expect(screen.getByText("completed").className).toContain("min-h-6");
  });

  it("does not render disclosure controls for entries without body content", () => {
    render(
      <TranscriptSection
        detail={buildProviderSessionDetailResponse({
          transcript: [
            {
              order: 1,
              status: "completed",
              type: "tool_output",
            },
          ],
        })}
      />,
    );

    expect(screen.getByText("completed")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

function buildProviderSessionDetailResponse(
  overrides: Partial<ProviderSessionDetailResponse>,
): ProviderSessionDetailResponse {
  return {
    parse: {
      eventCount: 1,
      functionCalls: [],
      lineCount: 1,
      malformedLineCount: 0,
      parseErrors: [],
      reasoning: [],
      turns: [],
      unknownEventCount: 0,
      unknownEvents: [],
    },
    providerSession: {
      id: "sess_active",
      kind: "session_id",
      provider: "codex",
    },
    source: {
      relativePath: "2026/05/18/rollout-sess_active.jsonl",
      sizeBytes: 2048,
    },
    transcript: [],
    ...overrides,
  };
}
