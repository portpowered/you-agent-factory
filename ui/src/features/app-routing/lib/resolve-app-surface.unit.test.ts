// @vitest-environment node

import {
  CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH,
  PACKAGED_FACTORIES_HOSTED_PATH,
  PACKAGED_FACTORIES_PATH,
  resolveAppSurface,
} from "./resolve-app-surface";

describe("resolveAppSurface", () => {
  it.each([
    [CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH, "customer-factory-emulator-demos"],
    [PACKAGED_FACTORIES_PATH, "packaged-factories"],
    [PACKAGED_FACTORIES_HOSTED_PATH, "packaged-factories"],
    ["/", "dashboard"],
    ["/unknown", "dashboard"],
  ] as const)("maps %s to %s", (pathname, expected) => {
    expect(resolveAppSurface(pathname)).toBe(expected);
  });
});
