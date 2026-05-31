import {
  factoryRuntimeNotIdleTarget,
  staleFactoryVersionTarget,
} from "../../testing/factory-validation-target-fixtures";
import { getSessionFactory, saveSessionFactory } from "./api";
import { SessionFactoryAPIError } from "./errors";
import { sessionFactoryOperatorErrorMessages } from "./operator-errors";

const baseSaveParams = {
  sessionID: "~default",
  factory: {
    name: "Current Factory",
    workers: [],
    workstations: [],
    workTypes: [],
  },
} as const;

describe("getSessionFactory transport errors", () => {
  it("throws SessionFactoryAPIError for network failures", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockRejectedValue(new Error("connection reset")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
      name: "SessionFactoryAPIError",
    });
  });

  it("uses rejected message when error response omits message", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response("{}", {
            headers: {
              "Content-Type": "application/json",
            },
            status: 500,
            statusText: "Internal Server Error",
          }),
        ),
      }),
    ).rejects.toBeInstanceOf(SessionFactoryAPIError);
  });
});

describe("saveSessionFactory stale and idle errors", () => {
  it("surfaces STALE_FACTORY_VERSION with structured targets", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "STALE_FACTORY_VERSION",
              message: "The session factory is stale.",
              targets: [
                staleFactoryVersionTarget("The session factory is stale."),
              ],
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 409,
              statusText: "Conflict",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      message: sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION,
      name: "SessionFactoryAPIError",
    });
  });

  it("surfaces FACTORY_NOT_IDLE", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "FACTORY_NOT_IDLE",
              message: "Factory runtime must be idle before activation.",
              targets: [
                factoryRuntimeNotIdleTarget(
                  "Factory runtime must be idle before activation.",
                ),
              ],
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 409,
              statusText: "Conflict",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
      message: sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE,
      name: "SessionFactoryAPIError",
    });
  });
});

describe("saveSessionFactory validation errors", () => {
  it("surfaces INVALID_FACTORY", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "INVALID_FACTORY",
              message: "The factory payload is invalid.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY",
      message: sessionFactoryOperatorErrorMessages.INVALID_FACTORY,
      name: "SessionFactoryAPIError",
    });
  });

  it("surfaces INVALID_FACTORY_NAME", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "INVALID_FACTORY_NAME",
              message: "The factory name is invalid.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY_NAME",
      message: sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME,
      name: "SessionFactoryAPIError",
    });
  });
});

describe("saveSessionFactory unknown error codes", () => {
  it("surfaces BAD_REQUEST and maps unknown codes to INTERNAL_ERROR", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "BAD_REQUEST",
              message: "The request payload is invalid.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "BAD_REQUEST",
      name: "SessionFactoryAPIError",
    });

    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "UNEXPECTED_SERVER_CODE",
              message: "Something went wrong.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 500,
              statusText: "Internal Server Error",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      name: "SessionFactoryAPIError",
    });
  });
});
