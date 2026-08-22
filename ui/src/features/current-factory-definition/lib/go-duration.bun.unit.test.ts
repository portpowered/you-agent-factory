import { afterEach, describe, expect, it, mock, spyOn } from "bun:test";

import { parseGoDurationNanoseconds } from "./go-duration";

afterEach(() => {
  mock.restore();
});

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
    expect(parseGoDurationNanoseconds("   ")).toBeNull();
  });

  it("parses fractional and microsecond duration units", () => {
    expect(parseGoDurationNanoseconds("1.5s")).toBe(1.5 * 1_000_000_000);
    expect(parseGoDurationNanoseconds("250ms")).toBe(250 * 1_000_000);
    expect(parseGoDurationNanoseconds("500µs")).toBe(500 * 1_000);
    expect(parseGoDurationNanoseconds("500μs")).toBe(500 * 1_000);
  });

  it("rejects non-finite parsed amounts", () => {
    spyOn(Number, "parseFloat").mockReturnValue(Number.NaN);

    expect(parseGoDurationNanoseconds("5s")).toBeNull();
  });
});
