/**
 * Stable semantic families used by every Factory graph renderer.
 *
 * The family is intentionally separate from a rendered node type. A renderer
 * may use `statePosition` or `workType` as its library-specific type while
 * the Factory graph still has one `work-state` or `work-type` family.
 */
export const FACTORY_GRAPH_NODE_FAMILIES = [
    "constraint",
    "doc",
    "resource",
    "worker",
    "work-state",
    "work-type",
    "workstation",
];
const FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS = {
    constraint: {
        allowedAxes: { height: false, width: true },
        defaultDimensions: { height: 58, width: 156 },
        family: "constraint",
        maximumDimensions: { height: 176, width: 360 },
        minimumDimensions: { height: 58, width: 128 },
        shape: "constraint",
    },
    doc: {
        allowedAxes: { height: true, width: true },
        defaultDimensions: { height: 86, width: 168 },
        family: "doc",
        maximumDimensions: { height: 320, width: 480 },
        minimumDimensions: { height: 86, width: 168 },
        shape: "document",
    },
    resource: {
        allowedAxes: { height: false, width: true },
        defaultDimensions: { height: 86, width: 168 },
        family: "resource",
        maximumDimensions: { height: 240, width: 420 },
        minimumDimensions: { height: 86, width: 168 },
        shape: "resource",
    },
    worker: {
        allowedAxes: { height: false, width: true },
        defaultDimensions: { height: 58, width: 156 },
        family: "worker",
        maximumDimensions: { height: 144, width: 360 },
        minimumDimensions: { height: 58, width: 156 },
        shape: "worker",
    },
    "work-state": {
        allowedAxes: { height: false, width: true },
        defaultDimensions: { height: 86, width: 164 },
        family: "work-state",
        maximumDimensions: { height: 240, width: 440 },
        minimumDimensions: { height: 86, width: 164 },
        shape: "work-state",
    },
    "work-type": {
        allowedAxes: { height: false, width: true },
        defaultDimensions: { height: 58, width: 156 },
        family: "work-type",
        maximumDimensions: { height: 144, width: 360 },
        minimumDimensions: { height: 58, width: 156 },
        shape: "work-type",
    },
    workstation: {
        allowedAxes: { height: true, width: true },
        defaultDimensions: { height: 196, width: 156 },
        family: "workstation",
        maximumDimensions: { height: 720, width: 520 },
        minimumDimensions: { height: 196, width: 156 },
        shape: "workstation",
    },
};
/** Read-only family table for package consumers that need the full role. */
export const FACTORY_GRAPH_NODE_FAMILY_ROLES = FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS;
export function factoryGraphNodeFamilyRole(family) {
    const role = FACTORY_GRAPH_NODE_FAMILY_ROLE_DEFINITIONS[family];
    return {
        ...role,
        allowedAxes: { ...role.allowedAxes },
        maximumDimensions: { ...role.maximumDimensions },
        defaultDimensions: { ...role.defaultDimensions },
        minimumDimensions: { ...role.minimumDimensions },
    };
}
export function factoryGraphNodeFamilyDimensions(family) {
    return { ...factoryGraphNodeFamilyRole(family).defaultDimensions };
}
/** Resolve a deterministic, family-bounded size from content or authored data. */
export function resolveFactoryGraphNodeDimensions(family, request) {
    const role = factoryGraphNodeFamilyRole(family);
    const options = normalizeDimensionRequest(request);
    const fittedDimensions = fitFactoryGraphNodeDimensionsForRole(role, options.content);
    const resolvedDimensions = resolveAuthoredDimensions(role, fittedDimensions, options.authoredDimensions);
    const source = options.authoredDimensions
        ? isFinitePositiveDimensions(options.authoredDimensions)
            ? "resolved"
            : "fitted"
        : options.content
            ? dimensionsEqual(fittedDimensions, role.defaultDimensions)
                ? "default"
                : "fitted"
            : "default";
    return {
        allowedAxes: { ...role.allowedAxes },
        bounds: {
            maximum: { ...role.maximumDimensions },
            minimum: { ...role.minimumDimensions },
        },
        defaultDimensions: { ...role.defaultDimensions },
        fittedDimensions,
        maximumDimensions: { ...role.maximumDimensions },
        minimumDimensions: { ...role.minimumDimensions },
        resolvedDimensions,
        source,
    };
}
export function fitFactoryGraphNodeDimensions(family, content) {
    return resolveFactoryGraphNodeDimensions(family, { content })
        .fittedDimensions;
}
/** Normalize an interactive resize request to the family's safe geometry. */
export function resolveFactoryGraphNodeResizeDimensions(family, requestedDimensions) {
    const resolution = resolveFactoryGraphNodeDimensions(family, {
        authoredDimensions: requestedDimensions,
    });
    const role = factoryGraphNodeFamilyRole(family);
    return {
        height: role.allowedAxes.height
            ? resolution.resolvedDimensions.height
            : resolution.fittedDimensions.height,
        width: role.allowedAxes.width
            ? resolution.resolvedDimensions.width
            : resolution.fittedDimensions.width,
    };
}
function normalizeDimensionRequest(request) {
    if (!request)
        return {};
    if (hasDimensionFields(request))
        return { authoredDimensions: request };
    return request;
}
function hasDimensionFields(request) {
    return "width" in request || "height" in request;
}
function fitFactoryGraphNodeDimensionsForRole(role, content) {
    const labels = normalizeContent(content);
    if (labels.length === 0)
        return { ...role.defaultDimensions };
    const contentPadding = contentPaddingForFamily(role.family);
    const width = clamp(Math.max(role.defaultDimensions.width, Math.ceil(Math.max(...labels.map(estimatedTextWidth)) + contentPadding)), role.minimumDimensions.width, role.maximumDimensions.width);
    const availableTextWidth = Math.max(1, width - contentPadding);
    const lineCount = labels.reduce((total, label) => total + wrappedLineCount(label, availableTextWidth), 0);
    const height = clamp(Math.max(role.defaultDimensions.height, role.defaultDimensions.height +
        Math.max(0, lineCount - baselineLineCount(role.family)) * 16), role.minimumDimensions.height, role.maximumDimensions.height);
    return { height, width };
}
function resolveAuthoredDimensions(role, fittedDimensions, authoredDimensions) {
    if (!authoredDimensions || !isFinitePositiveDimensions(authoredDimensions)) {
        return { ...fittedDimensions };
    }
    return {
        height: clamp(authoredDimensions.height, role.minimumDimensions.height, role.maximumDimensions.height),
        width: clamp(authoredDimensions.width, role.minimumDimensions.width, role.maximumDimensions.width),
    };
}
function normalizeContent(content) {
    const labels = typeof content === "string" ? [content] : (content ?? []);
    return labels
        .filter((label) => typeof label === "string")
        .map((label) => label.trim())
        .filter((label) => label.length > 0);
}
function isFinitePositiveDimensions(dimensions) {
    return (Number.isFinite(dimensions.width) &&
        Number.isFinite(dimensions.height) &&
        dimensions.width > 0 &&
        dimensions.height > 0);
}
function dimensionsEqual(left, right) {
    return left.width === right.width && left.height === right.height;
}
function clamp(value, minimum, maximum) {
    return Math.min(maximum, Math.max(minimum, value));
}
function contentPaddingForFamily(family) {
    return family === "workstation" ? 56 : family === "doc" ? 48 : 44;
}
function baselineLineCount(family) {
    return family === "workstation" ? 4 : family === "resource" ? 3 : 2;
}
function wrappedLineCount(label, availableWidth) {
    return label
        .split(/\r?\n/u)
        .reduce((total, line) => total +
        Math.max(1, Math.ceil(estimatedTextWidth(line) / availableWidth)), 0);
}
function estimatedTextWidth(label) {
    return [...label].reduce((total, character) => {
        if (/\s/u.test(character))
            return total + 4;
        if (isWideCharacter(character))
            return total + 14;
        if (/[ilIjtfr.,:'`|!]/u.test(character))
            return total + 4.5;
        if (/[A-Z0-9]/u.test(character))
            return total + 8;
        return total + 7;
    }, 0);
}
function isWideCharacter(character) {
    const codePoint = character.codePointAt(0) ?? 0;
    return ((codePoint >= 0x1100 && codePoint <= 0x11ff) ||
        (codePoint >= 0x2e80 && codePoint <= 0x9fff) ||
        (codePoint >= 0xac00 && codePoint <= 0xd7ff) ||
        (codePoint >= 0xf900 && codePoint <= 0xfaff) ||
        (codePoint >= 0x1f300 && codePoint <= 0x1faff));
}
const FAMILY_BY_SHELL_TYPE = {
    constraint: "constraint",
    doc: "doc",
    resource: "resource",
    statePosition: "work-state",
    worker: "worker",
    workType: "work-type",
    workstation: "workstation",
};
export function factoryGraphNodeFamilyForShellType(nodeType) {
    return FAMILY_BY_SHELL_TYPE[nodeType];
}
