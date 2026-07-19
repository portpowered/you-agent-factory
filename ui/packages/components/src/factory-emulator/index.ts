/** Controlled, host-state-owned Factory emulator visualizer components. */
/** Stable category path for `@you-agent-factory/components/factory-emulator`. */
export const COMPONENTS_CATEGORY = "factory-emulator" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export { FactoryEmulatorControls } from "./factory-emulator-controls";
export type {
  FactoryEmulatorAction,
  FactoryEmulatorControlsProps,
  FactoryEmulatorRuntimeStatus,
  FactoryEmulatorSpeed,
} from "./factory-emulator-controls";
