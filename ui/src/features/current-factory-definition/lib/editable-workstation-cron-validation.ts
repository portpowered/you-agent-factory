import parser from "cron-parser";
import { parseGoDurationNanoseconds } from "./go-duration";
import type { EditableWorkstationCronDraft } from "./workstation-editable-values";

const KNOWN_CRON_DESCRIPTOR_MACROS = new Set([
  "@annually",
  "@daily",
  "@hourly",
  "@midnight",
  "@monthly",
  "@weekly",
  "@yearly",
]);

export interface EditableWorkstationCronValidationMessages {
  cronExpiryWindowInvalid: (value: string) => string;
  cronJitterInvalid: (value: string) => string;
  cronScheduleRequired: string;
}

export type EditableWorkstationCronValidationErrors = {
  cronExpiryWindow?: string;
  cronJitter?: string;
  cronSchedule?: string;
};

export function validateEditableWorkstationCronDraft(
  cron: EditableWorkstationCronDraft | null,
  messages: EditableWorkstationCronValidationMessages,
): EditableWorkstationCronValidationErrors {
  if (!cron) {
    return {
      cronSchedule: messages.cronScheduleRequired,
    };
  }

  const validationErrors: EditableWorkstationCronValidationErrors = {};
  const scheduleError = validateCronScheduleExpression(
    cron.schedule,
    messages.cronScheduleRequired,
  );
  if (scheduleError) {
    validationErrors.cronSchedule = scheduleError;
  }

  const jitter = cron.jitter.trim();
  if (jitter.length > 0) {
    const jitterNanoseconds = parseGoDurationNanoseconds(jitter);
    if (jitterNanoseconds === null || jitterNanoseconds < 0) {
      validationErrors.cronJitter = messages.cronJitterInvalid(jitter);
    }
  }

  const expiryWindow = cron.expiryWindow.trim();
  if (expiryWindow.length > 0) {
    const expiryNanoseconds = parseGoDurationNanoseconds(expiryWindow);
    if (expiryNanoseconds === null || expiryNanoseconds <= 0) {
      validationErrors.cronExpiryWindow =
        messages.cronExpiryWindowInvalid(expiryWindow);
    }
  }

  return validationErrors;
}

function validateCronScheduleExpression(
  schedule: string,
  requiredMessage: string,
): string | undefined {
  const trimmedSchedule = schedule.trim();
  if (trimmedSchedule.length === 0) {
    return requiredMessage;
  }

  if (trimmedSchedule.startsWith("@")) {
    return validateCronDescriptorSchedule(trimmedSchedule);
  }

  const fields = trimmedSchedule.split(/\s+/);
  if (fields.length !== 5) {
    return formatInvalidCronSchedule(
      schedule,
      `gocron: CronJob: crontab parse failure\nexpected exactly ${5} fields, found ${fields.length}: [${fields.join(" ")}]`,
    );
  }

  try {
    parser.parseExpression(trimmedSchedule);
    return undefined;
  } catch (error) {
    const detail =
      error instanceof Error ? error.message : "crontab parse failure";
    return formatInvalidCronSchedule(schedule, detail);
  }
}

function validateCronDescriptorSchedule(schedule: string): string | undefined {
  const normalizedSchedule = schedule.toLowerCase();
  if (KNOWN_CRON_DESCRIPTOR_MACROS.has(normalizedSchedule)) {
    return undefined;
  }

  if (normalizedSchedule.startsWith("@every ")) {
    const durationValue = schedule.slice("@every ".length).trim();
    const durationNanoseconds = parseGoDurationNanoseconds(durationValue);
    if (durationNanoseconds === null || durationNanoseconds <= 0) {
      return formatInvalidCronSchedule(
        schedule,
        `invalid @every duration ${JSON.stringify(durationValue)}`,
      );
    }
    return undefined;
  }

  const descriptor = schedule.split(/\s+/)[0] ?? schedule;
  return formatInvalidCronSchedule(
    schedule,
    `gocron: CronJob: crontab parse failure\nunrecognized descriptor: ${descriptor}`,
  );
}

function formatInvalidCronSchedule(schedule: string, detail: string): string {
  return `invalid cron schedule ${JSON.stringify(schedule)}: ${detail}`;
}
