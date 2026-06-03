import {
  goDurationFromWorkerTimeoutPicker,
  workerTimeoutPickerFromGoDuration,
} from "./worker-timeout-duration";

describe("workerTimeoutPickerFromGoDuration", () => {
  it("represents an empty timeout as not configured", () => {
    expect(workerTimeoutPickerFromGoDuration(null)).toEqual({
      amount: "",
      unit: "m",
    });
    expect(workerTimeoutPickerFromGoDuration("")).toEqual({
      amount: "",
      unit: "m",
    });
  });

  it("parses second, minute, and hour Go duration strings", () => {
    expect(workerTimeoutPickerFromGoDuration("30s")).toEqual({
      amount: "30",
      unit: "s",
    });
    expect(workerTimeoutPickerFromGoDuration("5m")).toEqual({
      amount: "5",
      unit: "m",
    });
    expect(workerTimeoutPickerFromGoDuration("1h")).toEqual({
      amount: "1",
      unit: "h",
    });
  });

  it("normalizes composite durations into a single picker unit", () => {
    expect(workerTimeoutPickerFromGoDuration("1h30m")).toEqual({
      amount: "90",
      unit: "m",
    });
  });
});

describe("goDurationFromWorkerTimeoutPicker", () => {
  it("clears timeout when the picker is empty", () => {
    expect(
      goDurationFromWorkerTimeoutPicker({
        amount: "",
        unit: "m",
      }),
    ).toBeNull();
  });

  it("formats configured picker values as Go duration strings", () => {
    expect(
      goDurationFromWorkerTimeoutPicker({
        amount: "30",
        unit: "s",
      }),
    ).toBe("30s");
    expect(
      goDurationFromWorkerTimeoutPicker({
        amount: "5",
        unit: "m",
      }),
    ).toBe("5m");
    expect(
      goDurationFromWorkerTimeoutPicker({
        amount: "1",
        unit: "h",
      }),
    ).toBe("1h");
  });
});
