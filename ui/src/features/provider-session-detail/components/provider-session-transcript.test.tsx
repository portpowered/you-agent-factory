import { render, screen } from "@testing-library/react";

import type { ProviderSessionDetailResponse } from "../../../api/provider-session-details";
import { TranscriptSection } from "./provider-session-transcript";

describe("TranscriptSection", () => {
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

    expect(screen.getAllByText("Encrypted reasoning").length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText("sealed-chatgpt-reasoning-blob")).toBeTruthy();
    expect(screen.queryByText("Encrypted reasoning content only.")).toBeNull();
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
