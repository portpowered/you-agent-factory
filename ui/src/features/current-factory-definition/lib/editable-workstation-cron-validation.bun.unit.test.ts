import parser from "cron-parser";
import { afterEach, describe, expect, it, mock, spyOn } from "bun:test";

import {
  type EditableWorkstationCronValidationMessages,
  validateEditableWorkstationCronDraft,
} from "./editable-workstation-cron-validation";

afterEach(() => {
  mock.restore();
});

const messages: EditableWorkstationCronValidationMessages = {
  cronExpiryWindowInvalid: (value) =>
    `expiry_window must be a positive duration, got ${JSON.stringify(value)}`,
  cronJitterInvalid: (value) =>
    `jitter must be a non-negative duration, got ${JSON.stringify(value)}`,
  cronScheduleInvalid: (schedule, detail) =>
    `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`,
  cronScheduleRequired: "cron workstation requires non-empty 'schedule'",
};

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

describe("validateEditableWorkstationCronDraft descriptor schedules", () => {
  const validCron = {
    schedule: "0 * * * *",
    triggerAtStart: false,
    jitter: "",
    expiryWindow: "",
  };

  it.each([
    "@annually",
    "@daily",
    "@hourly",
    "@midnight",
    "@monthly",
    "@weekly",
    "@yearly",
  ] as const)("accepts cron descriptor macro %s", (schedule) => {
    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule },
        messages,
      ),
    ).toEqual({});
  });

  it("uses a generic parse failure message when cron-parser throws a non-Error", () => {
    spyOn(parser, "parseExpression").mockImplementation(() => {
      throw "broken parser";
    });

    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "0 * * * *" },
        messages,
      ).cronSchedule,
    ).toBe('invalid cron schedule "0 * * * *": crontab parse failure');
  });

  it("accepts @every schedules with positive Go durations", () => {
    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "@every 5m" },
        messages,
      ),
    ).toEqual({});
  });

  it("rejects invalid @every durations and unrecognized descriptors", () => {
    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "@every bad" },
        messages,
      ).cronSchedule,
    ).toBe('invalid cron schedule "@every bad": invalid @every duration "bad"');

    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "@every 0s" },
        messages,
      ).cronSchedule,
    ).toBe('invalid cron schedule "@every 0s": invalid @every duration "0s"');

    expect(
      validateEditableWorkstationCronDraft(
        { ...validCron, schedule: "@unknown" },
        messages,
      ).cronSchedule,
    ).toContain("unrecognized descriptor: @unknown");
  });
});
