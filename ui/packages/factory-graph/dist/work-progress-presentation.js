export const FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM = 3;
export function factoryGraphWorkProgressMode(count) {
    if (count <= 0)
        return "empty";
    return count <= FACTORY_GRAPH_WORK_ITEM_MODE_MAXIMUM ? "items" : "total";
}
