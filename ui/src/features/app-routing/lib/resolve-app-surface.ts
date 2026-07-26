export const CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH = "/factory-emulator-demos";
export const PACKAGED_FACTORIES_PATH = "/packaged-factories";
export const PACKAGED_FACTORIES_HOSTED_PATH = `/dashboard/ui${PACKAGED_FACTORIES_PATH}`;

export type AppSurface =
  | "customer-factory-emulator-demos"
  | "dashboard"
  | "packaged-factories";

export function resolveAppSurface(pathname: string): AppSurface {
  if (pathname === CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH) {
    return "customer-factory-emulator-demos";
  }
  if (
    pathname === PACKAGED_FACTORIES_PATH ||
    pathname === PACKAGED_FACTORIES_HOSTED_PATH
  ) {
    return "packaged-factories";
  }
  return "dashboard";
}
