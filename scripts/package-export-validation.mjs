function normalizedPackedPath(file) {
	const path = typeof file === "string" ? file : file?.path;
	return typeof path === "string"
		? path.replaceAll("\\", "/").replace(/^package\//, "")
		: null;
}

function exportTargets(value) {
	if (typeof value === "string") {
		return [value.replace(/^\.\//, "")];
	}
	if (Array.isArray(value)) {
		return value.flatMap(exportTargets);
	}
	if (typeof value === "object" && value !== null) {
		return Object.values(value).flatMap(exportTargets);
	}
	return [];
}

function matchesTarget(path, target) {
	const wildcard = target.indexOf("*");
	if (wildcard === -1) {
		return path === target;
	}
	const prefix = target.slice(0, wildcard);
	const suffix = target.slice(wildcard + 1);
	return path.startsWith(prefix) && path.endsWith(suffix);
}

function packedPathSet(files) {
	return new Set((files ?? []).map(normalizedPackedPath).filter(Boolean));
}

export function assertPackedExportTargets(packageName, exports, files) {
	const packedPaths = packedPathSet(files);
	for (const target of new Set(exportTargets(exports))) {
		if (![...packedPaths].some((path) => matchesTarget(path, target))) {
			throw new Error(`${packageName} candidate omits export target ${target}`);
		}
	}
}

export function assertPackedRequiredFiles(packageName, requiredFiles, files) {
	const packedPaths = packedPathSet(files);
	for (const requiredFile of requiredFiles) {
		if (!packedPaths.has(requiredFile)) {
			throw new Error(`${packageName} candidate omits ${requiredFile}`);
		}
	}
}
