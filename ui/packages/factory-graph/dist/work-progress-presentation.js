export const FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM = 3;
export function factoryGraphWorkProgressMode(count, itemModeMaximum = FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM) {
    if (count <= 0)
        return "empty";
    return count <= itemModeMaximum ? "items" : "total";
}
