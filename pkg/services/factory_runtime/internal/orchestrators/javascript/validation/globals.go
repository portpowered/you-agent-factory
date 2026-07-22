package workflowvalidation

var supportedRootGlobals = map[string]struct{}{
	"meta":     {},
	"args":     {},
	"phase":    {},
	"log":      {},
	"workflow": {},
	"agent":    {},
	"parallel": {},
	"pipeline": {},
}

var supportedWorkflowMembers = map[string]struct{}{
	"log":         {},
	"artifact":    {},
	"checkpoint":  {},
	"resumeState": {},
	"budget":      {},
	"final":       {},
}

var supportedAgentMembers = map[string]struct{}{
	"run": {},
}

var forbiddenRootGlobals = map[string]string{
	"require":        "Node/Bun require() module loading is not supported in MVP workflows",
	"import":         "ES module import is not supported in MVP workflows",
	"fs":             "direct filesystem access is not supported in MVP workflows",
	"child_process":  "direct process access is not supported in MVP workflows",
	"process":        "direct process access is not supported in MVP workflows",
	"net":            "direct network access is not supported in MVP workflows",
	"http":           "direct network access is not supported in MVP workflows",
	"https":          "direct network access is not supported in MVP workflows",
	"dgram":          "direct network access is not supported in MVP workflows",
	"dns":            "direct network access is not supported in MVP workflows",
	"tls":            "direct network access is not supported in MVP workflows",
	"fetch":          "direct network access is not supported in MVP workflows",
	"XMLHttpRequest": "direct network access is not supported in MVP workflows",
	"Bun":            "Bun host APIs are not supported in MVP workflows",
	"bun":            "Bun host APIs are not supported in MVP workflows",
	"Deno":           "Deno host APIs are not supported in MVP workflows",
	"npm":            "package-manager access is not supported in MVP workflows",
	"pnpm":           "package-manager access is not supported in MVP workflows",
	"yarn":           "package-manager access is not supported in MVP workflows",
	"eval":           "dynamic code execution is not supported in MVP workflows",
	"Function":       "dynamic code execution is not supported in MVP workflows",
	"__dirname":      "Node host paths are not supported in MVP workflows",
	"__filename":     "Node host paths are not supported in MVP workflows",
}
