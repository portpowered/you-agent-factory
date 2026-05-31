import {
  factoryRuntimeNotIdleTarget,
  staleFactoryVersionTarget,
} from "../../testing/factory-validation-target-fixtures";
import { saveSessionFactory } from "./api";
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
