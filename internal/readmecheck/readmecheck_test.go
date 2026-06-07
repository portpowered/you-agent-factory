package readmecheck

import (
	"slices"
	"testing"
)

func TestMissingRequiredSectionsDetectsAbsentHeadings(t *testing.T) {
	t.Parallel()

	content := `## Installation
## Features
`
	missing := MissingRequiredSections(content)
	want := []string{"Quick start", "Comparison", "References", "License"}
	if !slices.Equal(missing, want) {
		t.Fatalf("MissingRequiredSections() = %v, want %v", missing, want)
	}
}

func TestMissingRequiredSectionsPassesCompleteStructure(t *testing.T) {
	t.Parallel()

	content := `## Installation
## Quick start
## Features
## Comparison
## References
## License
`
	missing := MissingRequiredSections(content)
	if len(missing) != 0 {
		t.Fatalf("MissingRequiredSections() = %v, want none", missing)
	}
}

func TestLocalReferencePathsCollectsMarkdownAndHTMLAssets(t *testing.T) {
	t.Parallel()

	content := `# infinite-you
![hero](./docs/internal/resources/dashboard.png)
[License](./LICENSE.md)
![](docs/internal/resources/dashboard.gif)
<img src="examples/factories/ralph.png" alt="Ralph" />
[External](https://example.com)
[Anchor](#installation)
`
	paths := LocalReferencePaths(content)
	want := []string{
		"LICENSE.md",
		"docs/internal/resources/dashboard.gif",
		"docs/internal/resources/dashboard.png",
		"examples/factories/ralph.png",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("LocalReferencePaths() = %v, want %v", paths, want)
	}
}
