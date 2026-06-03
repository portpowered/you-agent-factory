import { parseGoDurationNanoseconds } from "./go-duration";

export type WorkerTimeoutUnit = "s" | "m" | "h";

export const WORKER_TIMEOUT_UNITS: WorkerTimeoutUnit[] = ["s", "m", "h"];

export interface WorkerTimeoutPickerValue {
  amount: string;
  unit: WorkerTimeoutUnit;
}

const NANOSECONDS_PER_SECOND = 1_000_000_000;
const SECONDS_PER_MINUTE = 60;
const SECONDS_PER_HOUR = 60 * SECONDS_PER_MINUTE;

export function workerTimeoutPickerFromGoDuration(
  timeout: string | null | undefined,
): WorkerTimeoutPickerValue {
  if (!timeout || timeout.trim().length === 0) {
    return { amount: "", unit: "m" };
  }

  const nanoseconds = parseGoDurationNanoseconds(timeout);
  if (nanoseconds === null || nanoseconds < 0) {
    return { amount: "", unit: "m" };
  }

  const totalSeconds = nanoseconds / NANOSECONDS_PER_SECOND;
  const display = pickWorkerTimeoutDisplay(totalSeconds);

  return {
    amount: formatWorkerTimeoutAmount(display.amount),
    unit: display.unit,
  };
}

export function goDurationFromWorkerTimeoutPicker(
  picker: WorkerTimeoutPickerValue,
): string | null {
  const trimmedAmount = picker.amount.trim();
  if (trimmedAmount.length === 0) {
    return null;
  }

  const amount = Number(trimmedAmount);
  if (!Number.isFinite(amount) || amount <= 0) {
    return null;
  }

  return `${formatWorkerTimeoutAmount(amount)}${picker.unit}`;
}

function pickWorkerTimeoutDisplay(totalSeconds: number): {
  amount: number;
  unit: WorkerTimeoutUnit;
} {
  if (
    totalSeconds >= SECONDS_PER_HOUR &&
    Number.isInteger(totalSeconds / SECONDS_PER_HOUR)
  ) {
    return {
      amount: totalSeconds / SECONDS_PER_HOUR,
      unit: "h",
    };
  }

  if (
    totalSeconds >= SECONDS_PER_MINUTE &&
    Number.isInteger(totalSeconds / SECONDS_PER_MINUTE)
  ) {
    return {
      amount: totalSeconds / SECONDS_PER_MINUTE,
      unit: "m",
    };
  }

  return {
    amount: totalSeconds,
    unit: "s",
  };
}

function formatWorkerTimeoutAmount(amount: number): string {
  if (Number.isInteger(amount)) {
    return String(amount);
  }

  return String(amount);
}
