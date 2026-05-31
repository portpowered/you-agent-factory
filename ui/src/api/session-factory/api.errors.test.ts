import {
  factoryRuntimeNotIdleTarget,
  staleFactoryVersionTarget,
} from "../../testing/factory-validation-target-fixtures";
import { saveSessionFactory } from "./api";

const baseSaveParams = {
  sessionID: "~default",
  factory: {
    name: "Current Factory",
    workers: [],
    workstations: [],
    workTypes: [],
  },
} as const;

describe("saveSessionFactory error mapping", () => {
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
      name: "SessionFactoryAPIError",
    });
  });

  it("surfaces BAD_REQUEST with the API message", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "BAD_REQUEST",
              message: "Factory name is required.",
            }),
            {
              headers: { "Content-Type": "application/json" },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "BAD_REQUEST",
      message: "Factory name is required.",
      name: "SessionFactoryAPIError",
    });
  });

  it("surfaces FACTORY_ALREADY_EXISTS with the API message", async () => {
    await expect(
      saveSessionFactory(baseSaveParams, {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "FACTORY_ALREADY_EXISTS",
              message: "Named factory already exists in this session.",
            }),
            {
              headers: { "Content-Type": "application/json" },
              status: 409,
              statusText: "Conflict",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "FACTORY_ALREADY_EXISTS",
      message: "Named factory already exists in this session.",
      name: "SessionFactoryAPIError",
    });
  });

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
      name: "SessionFactoryAPIError",
    });
  });
});
