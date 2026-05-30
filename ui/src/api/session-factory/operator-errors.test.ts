import { sessionFactoryAPIErrorMessages } from "./messages";
import {
  resolveSessionFactoryAPIErrorMessage,
  sessionFactoryOperatorErrorMessages,
} from "./operator-errors";
import { getSessionFactory, saveSessionFactory } from "./api";

const sessionFactoryFixture = {
  name: "Current Factory",
  workers: [],
  workstations: [],
  workTypes: [],
  version: {
    logical: "7",
    physical: "2026-05-18T14:22:00Z",
  },
};

describe("resolveSessionFactoryAPIErrorMessage", () => {
  it.each([
    ["STALE_FACTORY_VERSION", sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION],
    ["FACTORY_NOT_IDLE", sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE],
    ["INVALID_FACTORY", sessionFactoryOperatorErrorMessages.INVALID_FACTORY],
    ["INVALID_FACTORY_NAME", sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME],
  ] as const)(
    "maps %s to the canonical operator message regardless of API copy",
    (code, expectedMessage) => {
      expect(
        resolveSessionFactoryAPIErrorMessage({
          apiMessage: "Backend-specific diagnostic text.",
          code,
          rejectedMessage: sessionFactoryAPIErrorMessages.rejectedSaveRequest,
          status: 409,
        }),
      ).toBe(expectedMessage);
    },
  );

  it("maps network failures to the session factory network copy", () => {
    expect(
      resolveSessionFactoryAPIErrorMessage({
        code: "NETWORK_ERROR",
        rejectedMessage: sessionFactoryAPIErrorMessages.rejectedSaveRequest,
      }),
    ).toBe(sessionFactoryAPIErrorMessages.network);
  });

  it("maps 5xx internal failures to the rejected request copy", () => {
    expect(
      resolveSessionFactoryAPIErrorMessage({
        code: "INTERNAL_ERROR",
        rejectedMessage: sessionFactoryAPIErrorMessages.rejectedSaveRequest,
        status: 500,
      }),
    ).toBe(sessionFactoryAPIErrorMessages.rejectedSaveRequest);
  });
});

describe("session factory HTTP error mapping", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each([
    [
      "STALE_FACTORY_VERSION",
      sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION,
    ],
    [
      "FACTORY_NOT_IDLE",
      sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE,
    ],
    [
      "INVALID_FACTORY",
      sessionFactoryOperatorErrorMessages.INVALID_FACTORY,
    ],
  ] as const)(
    "surfaces canonical %s copy from mocked PUT failures",
    async (code, message) => {
      await expect(
        saveSessionFactory(
          {
            sessionID: "~default",
            factory: sessionFactoryFixture,
          },
          {
            fetch: vi.fn().mockResolvedValue(
              new Response(
                JSON.stringify({
                  code,
                  message: "Ignored API diagnostic.",
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
          },
        ),
      ).rejects.toMatchObject({
        code,
        message,
        status: 409,
      });
    },
  );

  it("surfaces canonical not-idle copy from mocked GET failures", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "FACTORY_NOT_IDLE",
              message: "Ignored API diagnostic.",
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
      status: 409,
    });
  });
});
