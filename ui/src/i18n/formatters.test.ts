import { describe, expect, it } from "bun:test";

import {
  formatCount,
  formatDate,
  formatDateTime,
  formatDuration,
  formatList,
  formatNumber,
  formatPercent,
  formatRelativeTime,
  formatTime,
} from "./formatters";

const fixedTimestamp = Date.UTC(2026, 4, 18, 9, 30);

describe("shared locale date and time formatters", () => {
  it("formats dates and times with the requested locale", () => {
    expect(
      formatDate(fixedTimestamp, "en", {
        dateStyle: "long",
        timeZone: "UTC",
      }),
    ).toBe("May 18, 2026");
    expect(
      formatDate(fixedTimestamp, "zh-CN", {
        dateStyle: "long",
        timeZone: "UTC",
      }),
    ).toBe("2026年5月18日");
    expect(
      formatTime(fixedTimestamp, "zh-CN", {
        hour12: false,
        timeZone: "UTC",
      }),
    ).toBe("9:30");
    expect(
      formatDateTime(fixedTimestamp, "en", {
        timeZone: "UTC",
      }),
    ).toMatch(/^May 18, 2026(?:,| at) 9:30 AM$/);
    expect(
      formatDateTime(fixedTimestamp, "zh-CN", {
        timeZone: "UTC",
      }),
    ).toBe("2026年5月18日 09:30");
  });

  it("formats numbers, percentages, and lists with Intl APIs", () => {
    expect(formatNumber(12_345.6, "en")).toBe("12,345.6");
    expect(formatNumber(12_345.6, "zh-CN")).toBe("12,345.6");
    expect(
      formatPercent(0.375, "zh-CN", {
        maximumFractionDigits: 1,
      }),
    ).toBe("37.5%");
    expect(formatList(["Alpha", "Beta", "Gamma"], "en")).toBe(
      "Alpha, Beta, and Gamma",
    );
    expect(formatList(["Alpha", "Beta", "Gamma"], "zh-CN")).toBe(
      "Alpha、Beta和Gamma",
    );
  });

});

describe("shared locale duration and relative-time formatters", () => {
  it("formats count-sensitive labels, durations, and relative values", () => {
    expect(
      formatCount(
        1,
        {
          one: "task",
          other: "tasks",
        },
        "en",
      ),
    ).toBe("1 task");
    expect(
      formatCount(
        2,
        {
          one: "task",
          other: "tasks",
        },
        "en",
      ),
    ).toBe("2 tasks");
    expect(
      formatCount(
        2,
        {
          other: "个任务",
        },
        "zh-CN",
      ),
    ).toBe("2 个任务");
    expect(formatDuration(192_000, "en")).toBe("3m 12s");
    expect(formatDuration(192_000, "zh-CN")).toBe("3分 12秒");
    expect(formatDuration(192_000, "ja")).toBe("3分 12秒");
    expect(formatDuration(192_000, "ko")).toBe("3분 12초");
    expect(
      formatDuration(7_440_000, "en", {
        style: "verbose",
      }),
    ).toBe("2 hours, 4 minutes");
    expect(
      formatDuration(7_440_000, "zh-CN", {
        style: "verbose",
      }),
    ).toBe("2小时4分钟");
    expect(
      formatDuration(7_440_000, "ja", {
        style: "verbose",
      }),
    ).toBe("2 時間, 4 分");
    expect(
      formatDuration(7_440_000, "ko", {
        style: "verbose",
      }),
    ).toBe("2시간, 4분");
    expect(formatRelativeTime(-1, "day", "en")).toBe("yesterday");
    expect(formatRelativeTime(-1, "day", "zh-CN")).toBe("昨天");
  });

  it("returns explicit fallback copy for invalid timestamp and relative-time input", () => {
    expect(
      formatDateTime("not-a-date", "en", {
        fallback: "Unavailable",
      }),
    ).toBe("Unavailable");
    expect(
      formatTime(undefined, "zh-CN", {
        fallback: "不可用",
      }),
    ).toBe("不可用");
    expect(
      formatDuration(undefined, "en", {
        fallback: "Unavailable",
      }),
    ).toBe("Unavailable");
    expect(
      formatRelativeTime(Number.NaN, "minute", "zh-CN", {
        fallback: "不可用",
      }),
    ).toBe("不可用");
  });

  it("keeps explicit timezone overrides available for time-only and date-time rendering", () => {
    expect(
      formatTime(fixedTimestamp, "en", {
        hour12: false,
        timeZone: "UTC",
      }),
    ).toBe("09:30");
    expect(
      formatTime(fixedTimestamp, "en", {
        hour12: false,
        timeZone: "America/New_York",
      }),
    ).toBe("05:30");
    expect(
      formatDateTime(fixedTimestamp, "en", {
        timeZone: "America/New_York",
      }),
    ).toMatch(/^May 18, 2026(?:,| at) 5:30 AM$/);
  });

});

describe("shared locale fallback behavior", () => {
  it("falls back to English for unsupported locale inputs", () => {
    expect(
      formatDate(fixedTimestamp, "fr", {
        dateStyle: "long",
        timeZone: "UTC",
      }),
    ).toBe("May 18, 2026");
  });
});
