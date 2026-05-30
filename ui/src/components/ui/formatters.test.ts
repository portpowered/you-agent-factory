import { describe, expect, it } from "bun:test";

import type { DashboardWorkItemRef } from "../../api/dashboard/types";
import {
  formatDurationFromISO,
  formatLocalTimezoneContext,
  getLocalDateTimeDisplay,
  formatLocalDateTime,
  formatDurationMillis,
  formatDurationMillisVerbose,
  formatRelativeTimeFromISO,
  formatTimeOfDay,
  formatTraceOutcome,
  formatList,
  formatWorkItemLabel,
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

  it("does not leak invalid numeric duration values into display labels", () => {
    expect(formatDurationMillis(Number.NaN)).toBe("");
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

describe("formatRelativeTimeFromISO", () => {
  it("formats relative-time labels for english and zh-CN", () => {
    const now = Date.parse("2026-04-10T12:00:04.000Z");

    expect(
      formatRelativeTimeFromISO("2026-04-10T12:00:00.000Z", now, "en"),
    ).toBe("4 seconds ago");
    expect(
      formatRelativeTimeFromISO("2026-04-10T12:00:00.000Z", now, "zh-CN"),
    ).toBe("4秒钟前");
  });

  it("returns unavailable for invalid timestamps", () => {
    expect(formatRelativeTimeFromISO("not-a-date", Date.now())).toBe(
      "Unavailable",
    );
  });
});

describe("formatTimeOfDay", () => {
  it("delegates valid ISO timestamps to the canonical local date-time formatter", () => {
    const timestamp = "2026-04-10T18:16:00.000Z";
    expect(formatTimeOfDay(timestamp)).toBe(
      formatLocalDateTime(timestamp, "Unavailable"),
    );
    expect(formatTimeOfDay(timestamp, "zh-CN", "不可用")).toBe(
      formatLocalDateTime(timestamp, "不可用", "zh-CN"),
    );
  });

  it("returns the supplied unavailable label for invalid timestamps", () => {
    expect(formatTimeOfDay("not-a-date")).toBe("Unavailable");
    expect(formatTimeOfDay("not-a-date", "zh-CN", "不可用")).toBe("不可用");
  });
});

describe("formatTraceOutcome", () => {
  it("localizes the empty outcome fallback", () => {
    expect(formatTraceOutcome("", "en")).toBe("Unknown");
    expect(formatTraceOutcome("", "zh-CN")).toBe("未知");
  });

  it("keeps non-empty outcomes humanized from the raw status", () => {
    expect(formatTraceOutcome("PROVIDER_RATE_LIMIT")).toBe(
      "Provider Rate Limit",
    );
  });
});

describe("formatList", () => {
  it("localizes the empty list fallback", () => {
    expect(formatList([], "en")).toBe("None");
    expect(formatList(undefined, "zh-CN")).toBe("无");
  });

  it("joins present values without translating identifiers", () => {
    expect(formatList(["alpha", "beta"], "zh-CN")).toBe("alpha, beta");
  });
});

describe("formatLocalDateTime", () => {
  const sampleTimestamp = "2026-04-10T18:16:00.000Z";

  it("formats ISO timestamps as medium date plus short local time for the default locale", () => {
    expect(formatLocalDateTime(sampleTimestamp, "Unavailable")).toBe(
      new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(Date.parse(sampleTimestamp)),
    );
  });

  it("formats ISO timestamps as medium date plus short local time for zh-CN", () => {
    expect(formatLocalDateTime(sampleTimestamp, "不可用", "zh-CN")).toBe(
      new Intl.DateTimeFormat("zh-CN", {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(Date.parse(sampleTimestamp)),
    );
  });

  it("returns the explicit unavailable fallback for invalid or missing timestamps", () => {
    expect(formatLocalDateTime("not-a-date", "Unavailable")).toBe(
      "Unavailable",
    );
    expect(formatLocalDateTime(undefined, "Unavailable")).toBe("Unavailable");
    expect(formatLocalDateTime("   ", "Unavailable")).toBe("Unavailable");
  });
});

describe("getLocalDateTimeDisplay", () => {
  it("returns a shared display label and raw timestamp for valid local date-times", () => {
    expect(
      getLocalDateTimeDisplay(" 2026-04-10T18:16:00.000Z ", "Unavailable"),
    ).toEqual({
      label: formatLocalDateTime(
        "2026-04-10T18:16:00.000Z",
        "Unavailable",
      ),
      rawTimestamp: "2026-04-10T18:16:00.000Z",
    });
  });

  it("uses explicit missing and invalid timestamp fallbacks without exposing raw invalid values", () => {
    expect(
      getLocalDateTimeDisplay(undefined, "Unavailable", "en", {
        missingLabel: "No timestamp",
      }),
    ).toEqual({
      label: "No timestamp",
      rawTimestamp: null,
    });
    expect(getLocalDateTimeDisplay("not-a-date", "Unavailable")).toEqual({
      label: "Unavailable",
      rawTimestamp: null,
    });
  });
});

describe("formatLocalTimezoneContext", () => {
  it("exposes the resolved local timezone as concise context", () => {
    expect(formatLocalTimezoneContext("Timezone")).toBe(
      `Timezone: ${new Intl.DateTimeFormat().resolvedOptions().timeZone}`,
    );
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
