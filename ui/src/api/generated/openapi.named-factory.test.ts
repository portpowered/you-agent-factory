import { describe, expect, it } from "vitest";

import type { components, operations, paths } from "./openapi";

describe("generated named-factory OpenAPI types", () => {
  it("exposes typed operations, payloads, and machine-readable error codes", () => {
    const request: operations["createFactory"]["requestBody"]["content"]["application/json"] = {
      name: "customer-support-triage",
      factory: {
        workTypes: [
          {
            name: "task",
            states: [
              { name: "init", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
            ],
          },
        ],
        workers: [
          {
            executorProvider: "script_wrap",
            model: "claude-sonnet-4-20250514",
            modelProvider: "claude",
            name: "planner",
            type: "MODEL_WORKER",
          },
        ],
        workstations: [
          {
            inputs: [{ state: "init", workType: "task" }],
            name: "plan-task",
            outputs: [{ state: "done", workType: "task" }],
            worker: "planner",
          },
        ],
      },
    };
    const created: operations["createFactory"]["responses"][201]["content"]["application/json"] =
      request;
    const current: paths["/factory/~current"]["get"]["responses"][200]["content"]["application/json"] =
      created;
    const invalidName: components["schemas"]["ErrorResponse"]["code"] =
      "INVALID_FACTORY_NAME";
    const invalidFactory: components["schemas"]["ErrorResponse"]["code"] =
      "INVALID_FACTORY";
    const duplicateName: components["schemas"]["ErrorResponse"]["code"] =
      "FACTORY_ALREADY_EXISTS";
    const runtimeBusy: components["schemas"]["ErrorResponse"]["code"] =
      "FACTORY_NOT_IDLE";
    const badRequestFamily: components["schemas"]["ErrorResponse"]["family"] =
      "BAD_REQUEST";
    const conflictFamily: components["schemas"]["ErrorResponse"]["family"] =
      "CONFLICT";
    const notFoundFamily: components["schemas"]["ErrorResponse"]["family"] =
      "NOT_FOUND";
    const currentNotFound: paths["/factory/~current"]["get"]["responses"][404]["content"]["application/json"] =
      {
        code: "NOT_FOUND",
        family: "NOT_FOUND",
        message: "Current named factory not found.",
      };

    expect(current.name).toBe("customer-support-triage");
    expect(current.factory.workstations?.[0]?.worker).toBe("planner");
    expect(currentNotFound.code).toBe("NOT_FOUND");
    expect(currentNotFound.family).toBe("NOT_FOUND");
    expect([invalidName, invalidFactory, duplicateName, runtimeBusy]).toEqual([
      "INVALID_FACTORY_NAME",
      "INVALID_FACTORY",
      "FACTORY_ALREADY_EXISTS",
      "FACTORY_NOT_IDLE",
    ]);
    expect([badRequestFamily, conflictFamily, notFoundFamily]).toEqual([
      "BAD_REQUEST",
      "CONFLICT",
      "NOT_FOUND",
    ]);
  });
});
