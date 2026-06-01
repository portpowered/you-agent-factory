import { describe, expect, it } from "vitest";

import {
  type EditableWorkstationCronValidationMessages,
  validateEditableWorkstationCronDraft,
} from "./editable-workstation-cron-validation";
import { parseGoDurationNanoseconds } from "./go-duration";

const messages: EditableWorkstationCronValidationMessages = {
  cronExpiryWindowInvalid: (value) =>
    `expiry_window must be a positive duration, got ${JSON.stringify(value)}`,
  cronJitterInvalid: (value) =>
    `jitter must be a non-negative duration, got ${JSON.stringify(value)}`,
  cronScheduleInvalid: (schedule, detail) =>
    `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`,
  cronScheduleRequired: "cron workstation requires non-empty 'schedule'",
};

describe("parseGoDurationNanoseconds", () => {
  it("parses common Go duration strings", () => {
    expect(parseGoDurationNanoseconds("5m")).toBe(5 * 60 * 1_000_000_000);
    expect(parseGoDurationNanoseconds("1h30m")).toBe(90 * 60 * 1_000_000_000);
    expect(parseGoDurationNanoseconds("0s")).toBe(0);
    expect(parseGoDurationNanoseconds("-1s")).toBe(-1 * 1_000_000_000);
  });

  it("rejects invalid duration strings", () => {
    expect(parseGoDurationNanoseconds("not-a-duration")).toBeNull();
    expect(parseGoDurationNanoseconds("")).toBeNull();
  });
});

describe("validateEditableWorkstationCronDraft", () => {
  const validCron = {
    schedule: "0 * * * *",
    triggerAtStart: false,
    jitter: "",
    expiryWindow: "",
  };

  it("requires a cron draft object", () => {
    expect(validateEditableWorkstationCronDraft(null, messages)).toEqual({
      cronSchedule: messages.cronScheduleRequired,
    });
  });

  it("requires a non-empty schedule", () => {
    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "   " },
        messages,
      ),
    ).toEqual({
      cronSchedule: messages.cronScheduleRequired,
    });
  });

  it("rejects invalid five-field cron schedules with backend-aligned errors", () => {
    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "not a cron" },
        messages,
      ).cronSchedule,
    ).toBe(
      'invalid cron schedule "not a cron": gocron: CronJob: crontab parse failure\nexpected exactly 5 fields, found 3: [not a cron]',
    );

    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "60 * * * *" },
        messages,
      ).cronSchedule,
    ).toContain('invalid cron schedule "60 * * * *"');
  });

  it("rejects invalid jitter and expiry window durations", () => {
    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, jitter: "-1s" },
        messages,
      ),
    ).toEqual({
      cronJitter: 'jitter must be a non-negative duration, got "-1s"',
    });

    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, expiryWindow: "0s" },
        messages,
      ),
    ).toEqual({
      cronExpiryWindow: 'expiry_window must be a positive duration, got "0s"',
    });

    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, jitter: "not-a-duration", expiryWindow: "bad" },
        messages,
      ),
    ).toEqual({
      cronExpiryWindow: 'expiry_window must be a positive duration, got "bad"',
      cronJitter:
        'jitter must be a non-negative duration, got "not-a-duration"',
    });
  });

  it("accepts valid cron schedules, jitter, and expiry windows", () => {
    expect(validateEditableWorkstationCronDraft(validCron, messages)).toEqual(
      {},
    );

    expect(
      validateEditableWorkstationCronDraft(
        {
          ...validCron,
          jitter: "30s",
          expiryWindow: "5m",
          schedule: "@daily",
        },
        messages,
      ),
    ).toEqual({});
  });
});
