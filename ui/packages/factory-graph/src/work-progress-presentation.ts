export const FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM = 3;

export type FactoryGraphWorkProgressMode = "empty" | "items" | "total";

export function factoryGraphWorkProgressMode(
  count: number,
  itemModeMaximum = FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM,
): FactoryGraphWorkProgressMode {
  if (count <= 0) return "empty";
  return count <= itemModeMaximum ? "items" : "total";
}
