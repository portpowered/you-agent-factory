package edges

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestPackageDocumentsProcessEdgeArchitectureException(t *testing.T) {
	t.Parallel()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "definition.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse definition.go: %v", err)
	}
	if file.Doc == nil {
		t.Fatal("package edges is missing package documentation")
	}
	doc := file.Doc.Text()
	requiredPhrases := []string{
		"process-edge aggregator",
		"root.BuildProcess",
		"pkg/wire",
		"functional",
		"not a service locator",
		"Initializer",
	}
	for _, phrase := range requiredPhrases {
		if !strings.Contains(doc, phrase) {
			t.Errorf("package documentation must state the process-edge architecture exception; missing %q", phrase)
		}
	}
	if !strings.Contains(doc, "exact") || !strings.Contains(strings.ToLower(doc), "port") {
		t.Error("package documentation must tell constructed services to take exact ports rather than the broad Edges bag")
	}
}

func TestPackageOwnsOnlyTheEdgeAggregator(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read edges package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typed, ok := specification.(*ast.TypeSpec)
				if !ok || typed.Name.Name == "Edges" {
					continue
				}
				t.Errorf(
					"%s declares %s; external-effect contracts belong to their effect adapter and edges only aggregates them",
					entry.Name(),
					typed.Name.Name,
				)
			}
		}
	}
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestEdgesAggregateExactOwnerTypes(t *testing.T) {
	t.Parallel()

	type ownershipDecision struct {
		typeName string
		effect   string
	}
	expected := map[string]ownershipDecision{
		"CLIObserver":                                     {typeName: "platformprocess.CLIObserver", effect: "observe the detached customer CLI contract and parser result"},
		"PlatformProcessClock":                            {typeName: "platformprocess.Clock", effect: "measure subprocess cleanup deadlines and diagnostic durations"},
		"PlatformProcessCommandFactory":                   {typeName: "platformprocess.CommandFactory", effect: "create inert host subprocess commands"},
		"ProviderCommandRunner":                           {typeName: "platformprocess.CommandRunner", effect: "launch external provider CLI processes"},
		"AgyPTYHost":                                      {typeName: "platformpty.Host", effect: "open native PTY handles and supervise an attached subprocess"},
		"AgyPTYClock":                                     {typeName: "platformclock.Source", effect: "measure Agy PTY session deadlines and activity"},
		"HostedHTTPClient":                                {typeName: "automations.HostedLinearHTTPDoer", effect: "send Linear GraphQL network requests"},
		"HostedLinearEndpoint":                            {typeName: "string", effect: "select the external Linear GraphQL endpoint"},
		"HostedSecretResolver":                            {typeName: "automations.HostedLinearSecretResolver", effect: "resolve the Linear credential at the hosted adapter"},
		"HostedLinearCheckpointStore":                     {typeName: "automations.HostedLinearCheckpointStore", effect: "persist hosted Linear resume checkpoints atomically"},
		"HostedClock":                                     {typeName: "automations.HostedLinearClock", effect: "schedule hosted-poller waits"},
		"ModelAssetHTTPClient":                            {typeName: "models.AssetHTTPDoer", effect: "download external model assets"},
		"ModelAssetEndpoints":                             {typeName: "models.RuntimeAssetEndpoints", effect: "select external model-catalog and asset endpoints"},
		"ModelAssetHostPlatform":                          {typeName: "models.AssetHostPlatform", effect: "select managed-model assets compatible with the process host"},
		"ModelAssetMakeDirectories":                       {typeName: "models.AssetMakeDirectories", effect: "create model asset directories"},
		"ModelAssetInspectPath":                           {typeName: "models.AssetInspectPath", effect: "inspect model asset paths"},
		"ModelAssetResolveHomeDirectory":                  {typeName: "models.AssetResolveHomeDirectory", effect: "resolve the model asset home directory"},
		"ModelAssetWriteFile":                             {typeName: "models.AssetWriteFile", effect: "write model asset files"},
		"ModelAssetRenamePath":                            {typeName: "models.AssetRenamePath", effect: "rename model asset paths"},
		"ModelAssetRemovePath":                            {typeName: "models.AssetRemovePath", effect: "remove model asset paths"},
		"ModelAssetReadFile":                              {typeName: "models.AssetReadFile", effect: "read model asset files"},
		"ModelAssetReadDirectory":                         {typeName: "models.AssetReadDirectory", effect: "read model asset directories"},
		"ModelAssetCreateFile":                            {typeName: "models.AssetCreateFile", effect: "create model asset files"},
		"ModelAssetOpenFile":                              {typeName: "models.AssetOpenFile", effect: "open model asset files"},
		"ModelHostProcessLauncher":                        {typeName: "models.HostProcessLauncher", effect: "launch the managed model host process"},
		"ModelHostHTTPClient":                             {typeName: "models.HostHTTPDoer", effect: "probe and invoke the managed model host"},
		"ModelHostClock":                                  {typeName: "models.HostClock", effect: "schedule model-host readiness probes"},
		"ModelRuntimeCommandRunner":                       {typeName: "platformprocess.CommandRunner", effect: "launch local model-runtime commands"},
		"ModelRuntimeHTTPClient":                          {typeName: "models.RuntimeHTTPDoer", effect: "invoke the local model runtime over HTTP"},
		"ModelRuntimeInspectFile":                         {typeName: "models.RuntimeInspectFile", effect: "inspect local model-runtime files"},
		"ModelRuntimeTempDirectory":                       {typeName: "models.RuntimeTempDirectory", effect: "resolve the process temporary directory"},
		"ModelRuntimeCreateTempFile":                      {typeName: "models.RuntimeCreateTempFile", effect: "create local model-runtime temporary files"},
		"ModelInvocationArtifactFileSystem":               {typeName: "models.InvocationArtifactFileSystem", effect: "export streamed model-invocation artifacts"},
		"FactorySessionsWorkingDirectory":                 {typeName: "platformfilesystem.WorkingDirectory", effect: "resolve omitted Factory Session invocation targets"},
		"FactorySessionExecutionOpeningFileSystem":        {typeName: "factorysessions.ExecutionOpeningFileSystem", effect: "resolve omitted durable-execution project and fixture-catalog paths"},
		"FactorySessionDirectoryInspection":               {typeName: "factorysessions.DirectoryInspection", effect: "inspect Factory Session target directories"},
		"FactorySessionContractFixtureReader":             {typeName: "factorysessions.ContractFixtureReader", effect: "read explicitly selected deterministic Factory Session fixture catalogs"},
		"FactorySessionInvocationInputReader":             {typeName: "factorysessions.InvocationInputReader", effect: "read customer files referenced by Factory Session invocation arguments"},
		"FactorySessionReplayRecordingReader":             {typeName: "factorysessions.ReplayRecordingReader", effect: "read customer-selected portable Factory Session recordings"},
		"FactorySessionInitialWorkReader":                 {typeName: "factorysessions.InitialWorkReader", effect: "read customer-selected initial Factory Session Work requests"},
		"FactorySessionResolveHomeDirectory":              {typeName: "factorysessions.HomeDirectoryResolver", effect: "resolve Factory Session home-relative paths"},
		"FactorySessionResolveLogicalTargetSymlinks":      {typeName: "factorysessions.LogicalTargetResolveSymlinks", effect: "canonicalize Factory Session logical target paths"},
		"FactorySessionIDGenerator":                       {typeName: "factorysessions.SessionIDGenerator", effect: "generate opaque live and durable Factory Session identities"},
		"FactorySessionRuntimeInstanceIDGenerator":        {typeName: "factorysessions.RuntimeInstanceIDGenerator", effect: "generate opaque Factory Session runtime instance identities"},
		"FactorySessionResponseEventIDGenerator":          {typeName: "factorysessions.ResponseEventIDGenerator", effect: "generate opaque Factory Session response-event identities"},
		"FactorySessionCursorPersistenceFileSystem":       {typeName: "factorysessions.CursorPersistenceFileSystem", effect: "persist Factory Session reconnect cursors"},
		"FactorySessionCursorCreateTemporaryFile":         {typeName: "factorysessions.CursorPersistenceCreateTemporaryFile", effect: "create atomic Factory Session reconnect-cursor writes"},
		"FactorySessionRuntimePersistenceFileSystem":      {typeName: "factorysessions.RuntimePersistenceFileSystem", effect: "persist durable Factory Session runtime snapshots"},
		"FactoryRuntimeIDGenerator":                       {typeName: "factoryruntime.IDGenerator", effect: "generate opaque Factory Runtime identities"},
		"FactoryRuntimeDirectories":                       {typeName: "factoryruntime.RuntimeDirectoryFileSystem", effect: "materialize Factory Runtime input directories"},
		"FactoryRuntimeInputs":                            {typeName: "factoryruntime.InputFileSystem", effect: "read Factory Runtime input files"},
		"FactoryRuntimeInputDirectoryWalker":              {typeName: "factoryruntime.InputDirectoryWalker", effect: "traverse Factory Runtime input directories"},
		"FactoryRuntimeWorkflowSources":                   {typeName: "factoryruntime.WorkflowSourceFileSystem", effect: "read JavaScript workflow sources"},
		"FactoryRuntimeWorkflowSourceResolveSymlinks":     {typeName: "factoryruntime.WorkflowSourceResolveSymlinks", effect: "resolve JavaScript workflow source symlinks"},
		"FactoryRuntimeWorkflowHome":                      {typeName: "factoryruntime.WorkflowHomeResolver", effect: "resolve global JavaScript workflow roots"},
		"FactoryDefinitionPortableFileSystem":             {typeName: "portablefiles.FileSystem", effect: "inspect and materialize portable Factory Definition files"},
		"FactoryDefinitionLoadingFileSystem":              {typeName: "factorydefinitions.LoadingFileSystem", effect: "resolve and read effective Factory Definition sources"},
		"FactoryDefinitionClock":                          {typeName: "factorydefinitions.Clock", effect: "timestamp editable Factory Definition versions"},
		"FactoryDefinitionVersionFileSystem":              {typeName: "factorydefinitions.VersionFileSystem", effect: "inspect legacy unversioned Factory Definition files"},
		"FactoryDefinitionPackagedGoalPromptFileSystem":   {typeName: "factorydefinitions.PackagedGoalPromptFileSystem", effect: "read packaged Goal prompt files for drift checks"},
		"FactoryDefinitionPortableBundledFileInspection":  {typeName: "factorydefinitions.PortableBundledFileInspection", effect: "inspect resolved portable bundled-file sources"},
		"FactoryDefinitionRequiredToolPathLookup":         {typeName: "factorydefinitions.RequiredToolPathLookup", effect: "resolve declarative Factory Definition tools"},
		"FactoryDefinitionRequiredToolVersionProbe":       {typeName: "factorydefinitions.RequiredToolVersionProbe", effect: "probe declarative Factory Definition tool versions"},
		"FactoryDefinitionPersistenceFileSystem":          {typeName: "factorydefinitions.PersistenceFileSystem", effect: "stage and publish persisted Factory Definition directories"},
		"FactoryDefinitionDirectoryReplacementStore":      {typeName: "factorydefinitions.DirectoryReplacementStore", effect: "atomically replace persisted Factory Definition directories"},
		"FactoryDefinitionNamedPathFileSystem":            {typeName: "factorydefinitions.NamedPathFileSystem", effect: "resolve and persist Current Factory named paths"},
		"FactoryDefinitionNamedFactoryCatalogFileSystem":  {typeName: "factorydefinitions.NamedFactoryCatalogFileSystem", effect: "inspect and delete persisted named Factory catalog entries"},
		"FactoryDefinitionPackagedInstallationFileSystem": {typeName: "factorydefinitions.PackagedInstallationFileSystem", effect: "inspect packaged Factory installation targets"},
		"FactoryDefinitionAuthoredReaderFileSystem":       {typeName: "factorydefinitions.AuthoredLayoutReaderFileSystem", effect: "read authored Factory Definition layouts"},
		"FactoryDefinitionAuthoredWriterFileSystem":       {typeName: "factorydefinitions.AuthoredLayoutWriterFileSystem", effect: "materialize authored Factory Definition layouts"},
		"FactoryDefinitionScaffoldFileSystem":             {typeName: "factorydefinitions.ScaffoldFileSystem", effect: "materialize newly authored Factory scaffold files"},
		"FactoryDefinitionScaffoldOutput":                 {typeName: "factorydefinitions.ScaffoldOutput", effect: "write Factory scaffold output when an invocation-local stream is omitted"},
		"ProviderSessionFileSystem":                       {typeName: "providersessions.FileSystem", effect: "inspect and open provider-session storage"},
		"ProviderSessionResolveHomeDirectory":             {typeName: "providersessions.ResolveHomeDirectory", effect: "resolve default provider-session storage roots"},
		"ProviderSessionCodexWalkDirectory":               {typeName: "providersessions.CodexWalkDirectory", effect: "traverse Codex provider-session storage"},
		"ProviderSessionCodexResolveSymlinks":             {typeName: "providersessions.CodexResolveSymlinks", effect: "resolve Codex provider-session symlinks"},
		"ProviderSessionCursorWalkDirectory":              {typeName: "providersessions.CursorWalkDirectory", effect: "traverse Cursor provider-session storage"},
		"ProviderSessionCursorResolveSymlinks":            {typeName: "providersessions.CursorResolveSymlinks", effect: "resolve Cursor provider-session symlinks"},
		"ProviderSessionCursorOpenDatabase":               {typeName: "providersessions.CursorOpenSQLDatabase", effect: "open Cursor provider-session databases"},
		"ProviderSessionOperatingSystem":                  {typeName: "providersessions.OperatingSystem", effect: "select Cursor provider-session platform layout"},
		"WorkersFactoryDocsFileSystem":                    {typeName: "platformfilesystem.ReadFileTree", effect: "inspect and read Factory documentation for Worker prompt rendering"},
		"WorkersResolveSymlinks":                          {typeName: "workers.ResolveExecutableSymlinks", effect: "resolve provider executable symlinks before capability negotiation"},
		"WorkersExecutableLocator":                        {typeName: "platformprocess.ExecutableLocator", effect: "locate provider executables on the host search path"},
		"WorkersExecutableFileReader":                     {typeName: "platformfilesystem.ReadOpener", effect: "stream provider executable contents for capability fingerprints"},
		"WorkersOperatingSystem":                          {typeName: "workers.OperatingSystem", effect: "select Worker provider command platform behavior"},
		"WorkersWorktreeFileSystem":                       {typeName: "workers.WorktreeFileSystem", effect: "inspect and reserve Factory-local Worker worktree paths"},
		"WorkersWorktreeGit":                              {typeName: "workers.WorktreeGitCommander", effect: "execute Git worktree discovery and creation commands"},
		"WorkersAgentToolFileSystem":                      {typeName: "workers.AgentToolFileSystem", effect: "execute bounded Worker agent-tool filesystem operations"},
		"WorkersMockWorkersConfigFileSystem":              {typeName: "workers.MockWorkersConfigFileSystem", effect: "read customer-selected mock-worker configuration files"},
		"WorkersRetryRandomSource":                        {typeName: "platformrandom.Source", effect: "supply bounded entropy for Worker provider retry jitter"},
		"WorkersWorkstationFileSystem":                    {typeName: "platformfilesystem.ReadFileInspector", effect: "read authored Worker interpolation files and inspect workstation execution paths"},
		"WorkersProviderTemporaryFileSystem":              {typeName: "platformfilesystem.TemporaryFileSystem", effect: "materialize and clean up Cursor provider request files"},
		"OperatorSettingsFileSystem":                      {typeName: "operatorsettings.FileSystem", effect: "persist operator configuration"},
		"OperatorSettingsCreateTemporaryFile":             {typeName: "operatorsettings.CreateTemporaryFile", effect: "create atomic operator-configuration writes"},
		"OperatorSettingsIDGenerator":                     {typeName: "operatorsettings.IDGenerator", effect: "generate local backend-scope identities"},
		"SystemInitializationInspectPath":                 {typeName: "systeminitialization.InspectPath", effect: "inspect system-initialization configuration paths"},
		"SystemInitializationMigrationFileSystem":         {typeName: "systeminitialization.LegacyFactoryMigrationFileSystem", effect: "migrate customer-owned Factories from the retired global catalog"},

		"Clock":                          {typeName: "platformclock.Source", effect: "supply process time to runtime and automation adapters"},
		"SubmissionRecorder":             {typeName: "recordings.SubmissionRecorder", effect: "observe canonical submission recording"},
		"DispatchRecorder":               {typeName: "recordings.DispatchRecorder", effect: "observe canonical dispatch recording"},
		"RecordingMakeDirectories":       {typeName: "recordings.RecordingMakeDirectories", effect: "create portable recording directories"},
		"RecordingCreateTempFile":        {typeName: "recordings.RecordingCreateTemporaryFile", effect: "create portable recording temporary files"},
		"RecordingRemovePath":            {typeName: "recordings.RecordingRemovePath", effect: "remove portable recording temporary files"},
		"RecordingRenamePath":            {typeName: "recordings.RecordingRenamePath", effect: "publish portable recording files atomically"},
		"APIServerStarter":               {typeName: "platformhttpserver.Starter", effect: "bind and serve the external HTTP listener"},
		"BrowserOpener":                  {typeName: "platformbrowser.Opener", effect: "open a customer-facing URL in the host browser"},
		"InvocationMetricsRecorder":      {typeName: "factorysessions.InvocationMetricsRecorder", effect: "publish Factory Session invocation metrics"},
		"RuntimeHostObserver":            {typeName: "factorysessions.RuntimeHostObserver", effect: "observe Factory Session runtime-host lifecycle"},
		"FactoryVisualizationSink":         {typeName: "factoryvisualization.Sink", effect: "present projected Factory visualization views at the process boundary"},
		"ModelPullMetricsRecorder":       {typeName: "models.PullMetricsRecorder", effect: "publish managed-model pull metrics"},
		"ProviderOverride":               {typeName: "providercontract.Provider", effect: "perform external provider inference"},
		"WorkersExecutablePathInspector": {typeName: "platformfilesystem.PathInspector", effect: "inspect the selected Worker executable path"},
		"ScriptCommandRunner":            {typeName: "platformprocess.CommandRunner", effect: "launch external script processes"},
		"WorkContentStagingFileSystem":   {typeName: "work.ContentStagingFileSystem", effect: "persist and clean up staged Work content"},
		"WorkContentStagingRandom":       {typeName: "work.ContentStagingRandom", effect: "generate staged Work signing keys and file names"},
		"WorkContentStagingClock":        {typeName: "work.ContentStagingClock", effect: "expire staged Work references"},
		"WorkContentHostPlatform":        {typeName: "work.ContentHostPlatform", effect: "select local Work content path conventions for the process host"},
		"WorkContentInspectPath":         {typeName: "work.ContentInspectPath", effect: "inspect local Work content paths"},
		"WorkContentCreateTempFile":      {typeName: "work.ContentCreateTemporaryFile", effect: "reserve temporary materialized Work content paths"},
		"WorkContentRemovePath":          {typeName: "work.ContentRemovePath", effect: "remove temporary materialized Work content paths"},
		"WorkContentWriteFile":           {typeName: "work.ContentWriteFile", effect: "write decoded inline Work content"},
		"WorkContentOpenFile":            {typeName: "work.ContentOpenFile", effect: "open temporary paths for remote Work content writes"},
		"WorkContentHTTPDoer":            {typeName: "work.ContentHTTPDoer", effect: "retrieve remote Work content"},
		"WorkRequestIDGenerator":         {typeName: "work.RequestIDGenerator", effect: "generate opaque Work Request identities"},
		"WorkSubmittedFileReader":        {typeName: "work.SubmittedFileReader", effect: "read submitted Work Request files"},
	}

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "definition.go", nil, 0)
	if err != nil {
		t.Fatalf("parse definition.go: %v", err)
	}
	actual := make(map[string]string)
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok || generic.Tok != token.TYPE {
			continue
		}
		for _, specification := range generic.Specs {
			typed, ok := specification.(*ast.TypeSpec)
			if !ok || typed.Name.Name != "Edges" {
				continue
			}
			structure, ok := typed.Type.(*ast.StructType)
			if !ok {
				t.Fatal("Edges is not a struct")
			}
			for _, field := range structure.Fields.List {
				var rendered bytes.Buffer
				if err := printer.Fprint(&rendered, fileSet, field.Type); err != nil {
					t.Fatalf("render Edges field type: %v", err)
				}
				for _, name := range field.Names {
					actual[name.Name] = rendered.String()
				}
			}
		}
	}
	for name, decision := range expected {
		if strings.TrimSpace(decision.effect) == "" {
			t.Errorf("Edges.%s has no identified external effect", name)
		}
		if got, ok := actual[name]; !ok {
			t.Errorf("Edges.%s is missing from ownership inventory", name)
		} else if got != decision.typeName {
			t.Errorf("Edges.%s type = %s, want exact owner type %s for %s", name, got, decision.typeName, decision.effect)
		}
		delete(actual, name)
	}
	for name, typeName := range actual {
		t.Errorf("Edges.%s type %s has no external-effect ownership decision", name, typeName)
	}
}
