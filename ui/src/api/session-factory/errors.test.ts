import {
  normalizeSessionFactoryAPIErrorCode,
  SessionFactoryAPIError,
} from "./errors";

describe("normalizeSessionFactoryAPIErrorCode", () => {
  it.each([
    "BAD_REQUEST",
    "FACTORY_ALREADY_EXISTS",
    "FACTORY_NOT_IDLE",
    "INVALID_FACTORY",
    "INVALID_FACTORY_NAME",
    "NOT_FOUND",
    "STALE_FACTORY_VERSION",
  ] as const)("returns %s for recognized API codes", (code) => {
    expect(normalizeSessionFactoryAPIErrorCode(code)).toBe(code);
  });

  it("maps unknown or missing API codes to INTERNAL_ERROR", () => {
    expect(normalizeSessionFactoryAPIErrorCode("UNKNOWN_CODE")).toBe("INTERNAL_ERROR");
    expect(normalizeSessionFactoryAPIErrorCode(undefined)).toBe("INTERNAL_ERROR");
  });
});

describe("SessionFactoryAPIError", () => {
  it("preserves structured error details on the thrown error", () => {
    const error = new SessionFactoryAPIError("Factory save failed.", {
      code: "FACTORY_ALREADY_EXISTS",
      responseBody: { code: "FACTORY_ALREADY_EXISTS" },
      status: 409,
      statusText: "Conflict",
    });

    expect(error).toMatchObject({
      code: "FACTORY_ALREADY_EXISTS",
      message: "Factory save failed.",
      name: "SessionFactoryAPIError",
      responseBody: { code: "FACTORY_ALREADY_EXISTS" },
      status: 409,
      statusText: "Conflict",
    });
  });
});
