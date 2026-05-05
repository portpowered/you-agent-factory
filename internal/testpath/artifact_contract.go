package testpath

type artifactClassification string

const (
	artifactCheckedIn artifactClassification = "checked_in"
	artifactGenerated artifactClassification = "generated"
	artifactObsolete  artifactClassification = "obsolete"
)

type artifactContractEntry struct {
	Path           string
	Classification artifactClassification
	Reason         string
}

var artifactContractEntries = []artifactContractEntry{
	{
		Path:           "factory",
		Classification: artifactCheckedIn,
		Reason:         "Canonical checked-in repository starter root.",
	},
	{
		Path:           "factory/README.md",
		Classification: artifactCheckedIn,
		Reason:         "Canonical checked-in repository starter documentation.",
	},
	{
		Path:           "factory/factory.json",
		Classification: artifactCheckedIn,
		Reason:         "Canonical checked-in repository starter config.",
	},
	{
		Path:           "factory/scripts/setup-workspace.py",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in workspace setup helper used by the canonical repository workflow.",
	},
	{
		Path:           "factory/inputs",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in starter input directories used by the repository-local workflow and backed by tracked sentinels.",
	},
	{
		Path:           "factory/inputs/BATCH/default",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical inbox for ordered or mixed-work-type FACTORY_REQUEST_BATCH submissions.",
	},
	{
		Path:           "factory/inputs/BATCH/default/.gitkeep",
		Classification: artifactCheckedIn,
		Reason:         "Tracked sentinel that keeps the canonical batch inbox present in clean checkouts.",
	},
	{
		Path:           "factory/inputs/idea/default",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in repository workflow idea inbox backed by a tracked sentinel.",
	},
	{
		Path:           "factory/inputs/idea/default/.gitkeep",
		Classification: artifactCheckedIn,
		Reason:         "Tracked sentinel that keeps the canonical idea inbox present in clean checkouts.",
	},
	{
		Path:           "factory/inputs/plan/default",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in repository workflow plan inbox backed by a tracked sentinel.",
	},
	{
		Path:           "factory/inputs/plan/default/.gitkeep",
		Classification: artifactCheckedIn,
		Reason:         "Tracked sentinel that keeps the canonical plan inbox present in clean checkouts.",
	},
	{
		Path:           "factory/inputs/task/default",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in repository workflow task inbox backed by a tracked sentinel.",
	},
	{
		Path:           "factory/inputs/task/default/.gitkeep",
		Classification: artifactCheckedIn,
		Reason:         "Tracked sentinel that keeps the canonical task inbox present in clean checkouts.",
	},
	{
		Path:           "factory/inputs/thoughts/default",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in repository workflow thought inbox backed by a tracked sentinel.",
	},
	{
		Path:           "factory/inputs/thoughts/default/.gitkeep",
		Classification: artifactCheckedIn,
		Reason:         "Tracked sentinel that keeps the canonical thought inbox present in clean checkouts.",
	},
	{
		Path:           "factory/logs/meta/asks.md",
		Classification: artifactCheckedIn,
		Reason:         "Canonical checked-in customer-ask backlog for the meta and cleaner workflow.",
	},
	{
		Path:           "factory/logs/meta/view.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in meta world-state view consumed by the cleaner workflow.",
	},
	{
		Path:           "factory/logs/meta/progress.txt",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in meta progress surface consumed by the cleaner workflow.",
	},
	{
		Path:           "factory/logs/agent-fails.json",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in event-stream sample used for replay conversion coverage.",
	},
	{
		Path:           "factory/logs/agent-fails.replay.json",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in replay artifact sample paired with the event-stream conversion smoke.",
	},
	{
		Path:           "factory/meta/asks.md",
		Classification: artifactCheckedIn,
		Reason:         "Redirect-only legacy stub that points maintainers back to the canonical checked-in meta ask surface.",
	},
	{
		Path:           "tests/adhoc/factory-recording-04-11-02.json",
		Classification: artifactCheckedIn,
		Reason:         "Canonical replay fixture used by targeted adhoc and replay tests.",
	},
	{
		Path:           "tests/adhoc/factory/README.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in adhoc fixture documentation.",
	},
	{
		Path:           "tests/adhoc/factory/factory.json",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in adhoc fixture config.",
	},
	{
		Path:           "ui/src/api/generated/openapi.ts",
		Classification: artifactGenerated,
		Reason:         "Generated TypeScript client types checked in from the authored OpenAPI contract.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/failure-analysis-events.ts",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/graph-state-smoke-events.ts",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/resource-count-events.ts",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "ui/src/components/dashboard/fixtures/runtime-details-events.ts",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in dashboard replay fixture.",
	},
	{
		Path:           "factory/workers/processor/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical processor worker prompt.",
	},
	{
		Path:           "factory/workers/workspace-setup/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical workspace setup worker prompt.",
	},
	{
		Path:           "factory/workstations/cleaner/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical repository cleanup workstation prompt.",
	},
	{
		Path:           "factory/workstations/ideafy/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical ideation workstation prompt.",
	},
	{
		Path:           "factory/workstations/plan/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical planning workstation prompt.",
	},
	{
		Path:           "factory/workstations/process/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical execution workstation prompt.",
	},
	{
		Path:           "factory/workstations/review/AGENTS.md",
		Classification: artifactCheckedIn,
		Reason:         "Checked-in canonical review workstation prompt.",
	},
	{
		Path:           "factory/workers/executor/AGENTS.md",
		Classification: artifactObsolete,
		Reason:         "Legacy story-starter worker path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/workers/reviewer/AGENTS.md",
		Classification: artifactObsolete,
		Reason:         "Legacy story-starter worker path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/workstations/execute-story/AGENTS.md",
		Classification: artifactObsolete,
		Reason:         "Legacy story-starter workstation path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/workstations/review-story/AGENTS.md",
		Classification: artifactObsolete,
		Reason:         "Legacy story-starter workstation path no longer belongs to the canonical root factory.",
	},
	{
		Path:           "factory/inputs/story/default/example-story.md",
		Classification: artifactObsolete,
		Reason:         "Legacy story-starter seed file is not part of the canonical root factory surface.",
	},
	{
		Path:           "factory/old/README.md",
		Classification: artifactObsolete,
		Reason:         "Legacy historical starter tree is not part of the canonical root factory surface.",
	},
}
