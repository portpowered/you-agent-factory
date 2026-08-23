import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

const liveEvents = new Set(["schedule", "workflow_dispatch"]);

export function selectPublishedBackendConformance({ eventName } = {}) {
	if (liveEvents.has(eventName)) {
		return {
			selected: true,
			reason: `${eventName} invokes the low-frequency published-backend live cell.`,
		};
	}
	return {
		selected: false,
		reason: "Pull-request and other events do not run live published-backend requests.",
	};
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url && process.env.GITHUB_EVENT_NAME) {
	const selection = selectPublishedBackendConformance({
		eventName: process.env.GITHUB_EVENT_NAME,
	});
	console.log(JSON.stringify(selection));
	if (!selection.selected) {
		process.exitCode = 1;
	}
}
