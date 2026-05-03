import { describe, expect, it } from "vitest";

import type { DashboardWorkItemRef } from "../../api/dashboard/types";
import {
  formatDurationFromISO,
  formatDurationMillis,
  formatDurationMillisVerbose,
  formatTimeOfDay,
  formatWorkItemLabel,
  formatWorkstationRunOutcome,
  getProviderSessionLogTarget,
} from "./formatters";

describe("formatDurationMillis", () => {
  it("formats durations in human-readable units", () => {
    expect(formatDurationMillis(450)).toBe("450ms");
    expect(formatDurationMillis(3_000)).toBe("3s");
    expect(formatDurationMillis(192_000)).toBe("3m 12s");
    expect(formatDurationMillis(7_440_000)).toBe("2h 4m");
  });

  it("clamps negative durations to zero milliseconds", () => {
    expect(formatDurationMillis(-100)).toBe("0ms");
  });
});

describe("formatDurationMillisVerbose", () => {
  it("formats durations with spelled-out units for human-readable detail rows", () => {
    expect(formatDurationMillisVerbose(875)).toBe("875 milliseconds");
    expect(formatDurationMillisVerbose(3_000)).toBe("3 seconds");
    expect(formatDurationMillisVerbose(192_000)).toBe("3 minutes, 12 seconds");
    expect(formatDurationMillisVerbose(7_440_000)).toBe("2 hours, 4 minutes");
  });

  it("omits empty trailing units", () => {
    expect(formatDurationMillisVerbose(7_200_000)).toBe("2 hours");
    expect(formatDurationMillisVerbose(180_000)).toBe("3 minutes");
  });
});

describe("formatDurationFromISO", () => {
  it("formats elapsed time from an ISO timestamp with the shared duration formatter", () => {
    expect(
      formatDurationFromISO(
        "2026-04-10T12:00:00.000Z",
        Date.parse("2026-04-10T12:00:00.450Z"),
      ),
    ).toBe("450ms");

    expect(
      formatDurationFromISO(
        "2026-04-10T12:00:00.000Z",
        Date.parse("2026-04-10T14:04:59.000Z"),
      ),
    ).toBe("2h 4m");
  });

  it("returns unavailable for invalid timestamps", () => {
    expect(formatDurationFromISO("not-a-date", Date.now())).toBe("Unavailable");
  });
});

describe("formatTimeOfDay", () => {
  it("formats ISO timestamps as local clock times for detail cards", () => {
    expect(formatTimeOfDay("2026-04-10T18:16:00.000Z")).toBe(
      new Intl.DateTimeFormat(undefined, {
        hour: "numeric",
        minute: "2-digit",
      })
        .format(Date.parse("2026-04-10T18:16:00.000Z"))
        .replace(/\s/g, ""),
    );
  });

  it("falls back to the raw value for invalid timestamps", () => {
    expect(formatTimeOfDay("not-a-date")).toBe("not-a-date");
  });
});

describe("formatWorkItemLabel", () => {
  it("falls back to the work id when the display name is blank", () => {
    expect(
      formatWorkItemLabel({
        display_name: "   ",
        work_id: "work-123",
      }),
    ).toBe("work-123");
  });

  it("returns a safe fallback when both display name and work id are missing", () => {
    expect(
      formatWorkItemLabel({
        trace_id: "trace-123",
      } as DashboardWorkItemRef),
    ).toBe("Unknown work");
  });
});

describe("formatWorkstationRunOutcome", () => {
  it("renders repeater rejection-loop outcomes as repeated work with raw outcome metadata", () => {
    expect(
      formatWorkstationRunOutcome("REJECTED", { workstationKind: "repeater" }),
    ).toEqual({
      label: "Repeated work",
      rawOutcomeLabel: "Raw outcome: REJECTED",
    });
  });

  it("keeps terminal rejection wording for non-repeater workstation outcomes", () => {
    expect(
      formatWorkstationRunOutcome("REJECTED", { workstationKind: "standard" }),
    ).toEqual({
      label: "Rejected",
    });
  });

  it("keeps accepted, failed, retry, and unknown outcomes readable", () => {
    expect(formatWorkstationRunOutcome("ACCEPTED", { workstationKind: "repeater" })).toEqual({
      label: "Accepted",
    });
    expect(formatWorkstationRunOutcome("FAILED", { workstationKind: "repeater" })).toEqual({
      label: "Failed",
    });
    expect(formatWorkstationRunOutcome("RETRY", { workstationKind: "repeater" })).toEqual({
      label: "Retry",
    });
    expect(formatWorkstationRunOutcome("", { workstationKind: "repeater" })).toEqual({
      label: "Unknown",
    });
  });
});

describe("getProviderSessionLogTarget", () => {
  it("uses an explicit session log URL when the scheme is safe", () => {
    expect(
      getProviderSessionLogTarget({
        id: "sess-1",
        provider: "codex",
        session_log_url: "http://127.0.0.1:8080/logs/sess-1.jsonl",
      }),
    ).toEqual({
      display: "http://127.0.0.1:8080/logs/sess-1.jsonl",
      href: "http://127.0.0.1:8080/logs/sess-1.jsonl",
    });
  });

  it("converts an explicit local JSONL path into a file link", () => {
    expect(
      getProviderSessionLogTarget({
        id: "sess-1",
        local_jsonl_path: "C:\\Users\\operator\\codex sessions\\sess-1.jsonl",
        provider: "codex",
      }),
    ).toEqual({
      display: "C:\\Users\\operator\\codex sessions\\sess-1.jsonl",
      href: "file:///C:/Users/operator/codex%20sessions/sess-1.jsonl",
    });
  });

  it("rejects unsafe URLs and non-JSONL local paths", () => {
    expect(
      getProviderSessionLogTarget({
        id: "sess-1",
        provider: "codex",
        session_log_url: "javascript:alert(1)",
      }),
    ).toBeNull();
    expect(
      getProviderSessionLogTarget({
        id: "sess-1",
        local_jsonl_path: "C:\\Users\\operator\\codex-sessions\\sess-1.txt",
        provider: "codex",
      }),
    ).toBeNull();
  });
});

