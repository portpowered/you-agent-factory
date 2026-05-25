import { describe, expect, it } from "vitest";

import {
  formatCount,
  formatDate,
  formatDateTime,
  formatList,
  formatNumber,
  formatPercent,
  formatRelativeTime,
  formatTime,
} from "./formatters";

const fixedTimestamp = Date.UTC(2026, 4, 18, 9, 30);

describe("shared locale formatters", () => {
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
    ).toBe("May 18, 2026, 9:30 AM");
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

  it("formats count-sensitive labels and relative values", () => {
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
    expect(formatRelativeTime(-1, "day", "en")).toBe("yesterday");
    expect(formatRelativeTime(-1, "day", "zh-CN")).toBe("昨天");
  });

  it("falls back to English for unsupported locale inputs", () => {
    expect(
      formatDate(fixedTimestamp, "fr", {
        dateStyle: "long",
        timeZone: "UTC",
      }),
    ).toBe("May 18, 2026");
  });
});
