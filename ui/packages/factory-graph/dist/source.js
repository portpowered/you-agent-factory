export function isFactoryGraphSource(value) {
    if (!value || typeof value !== "object")
        return false;
    const source = value;
    const selectedTick = source.selectedTick;
    return (source.factory !== undefined &&
        source.runtime !== undefined &&
        typeof selectedTick === "number" &&
        Number.isSafeInteger(selectedTick) &&
        selectedTick >= 0);
}
/** Validate the stable graph boundary without cloning or reducing its inputs. */
export function createFactoryGraphSource(source) {
    if (!isFactoryGraphSource(source)) {
        throw new TypeError("Factory graph source requires a Factory, runtime projection, and non-negative selected tick.");
    }
    return source;
}
