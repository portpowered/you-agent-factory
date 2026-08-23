export function selectBackendIntegration({ eventName = "", ref = "" } = {}) {
	const selected =
		eventName === "pull_request" ||
		(eventName === "push" && ref === "refs/heads/main");
	return { selected, error: "" };
}
