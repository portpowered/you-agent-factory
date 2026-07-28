package identityinputinventory

const (
	outcomeAccept = "accept"
	outcomeReject = "reject"

	entrypointDecodeGlobalConfig = "DecodeGlobalConfig"
	entrypointLoadFileConfig     = "LoadFileConfig"
	entrypointResolve            = "Resolve"

	categoryParseDefaults     = "parse-defaults"
	categoryParseWorkerPreset = "parse-worker-presets"
	categoryParseUnknownField = "parse-unknown-field"
	categoryLoadFile          = "load-file"
	categoryResolvePrecedence = "resolve-precedence"
	categoryResolveSymbolic   = "resolve-symbolic-default"
)
