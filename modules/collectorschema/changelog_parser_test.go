package collectorschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChangelog_AllSections(t *testing.T) {
	changelog := `## v0.135.0

### 🛑 Breaking changes 🛑

- ` + "`apachereceiver`" + `: Add number of connections per async state metrics. (#41886)
- ` + "`receiver/sapm`" + `: The SAPM Receiver component has been removed. (#41411)
  Additional details here as subtext.

### 🚩 Deprecations 🚩

- ` + "`k8sclusterreceiver`" + `: The namespace option is deprecated, use namespaces instead. (#40089)

### 🚀 New components 🚀

- ` + "`newreceiver`" + `: Added new receiver for testing. (#12345)

### 💡 Enhancements 💡

- ` + "`transformprocessor`" + `: Add support for merging histogram buckets. (#40280)

### 🧰 Bug fixes 🧰

- ` + "`libhoneyreceiver`" + `: Properly handle compressed payloads (#42279)
`

	parsed, err := ParseChangelog(changelog)
	require.NoError(t, err)

	// Breaking changes
	require.Len(t, parsed.BreakingChanges, 2)
	assert.Equal(t, "apachereceiver", parsed.BreakingChanges[0].Component)
	assert.Equal(t, "Add number of connections per async state metrics.", parsed.BreakingChanges[0].Note)
	assert.Equal(t, []string{"#41886"}, parsed.BreakingChanges[0].IssueRefs)

	assert.Equal(t, "receiver/sapm", parsed.BreakingChanges[1].Component)
	assert.Equal(t, "The SAPM Receiver component has been removed.", parsed.BreakingChanges[1].Note)
	assert.Equal(t, "Additional details here as subtext.", parsed.BreakingChanges[1].SubText)

	// Deprecations
	require.Len(t, parsed.Deprecations, 1)
	assert.Equal(t, "k8sclusterreceiver", parsed.Deprecations[0].Component)

	// New components
	require.Len(t, parsed.NewComponents, 1)
	assert.Equal(t, "newreceiver", parsed.NewComponents[0].Component)

	// Enhancements
	require.Len(t, parsed.Enhancements, 1)
	assert.Equal(t, "transformprocessor", parsed.Enhancements[0].Component)

	// Bug fixes
	require.Len(t, parsed.BugFixes, 1)
	assert.Equal(t, "libhoneyreceiver", parsed.BugFixes[0].Component)
}

func TestParseChangelog_WithoutEmojis(t *testing.T) {
	changelog := `## v0.135.0

### Breaking changes

- ` + "`component`" + `: Description here. (#12345)

### Bug fixes

- ` + "`othercomponent`" + `: Fixed something. (#54321)
`

	parsed, err := ParseChangelog(changelog)
	require.NoError(t, err)

	require.Len(t, parsed.BreakingChanges, 1)
	assert.Equal(t, "component", parsed.BreakingChanges[0].Component)

	require.Len(t, parsed.BugFixes, 1)
	assert.Equal(t, "othercomponent", parsed.BugFixes[0].Component)
}

func TestParseChangelog_MultipleIssueRefs(t *testing.T) {
	changelog := `### Breaking changes

- ` + "`component`" + `: Description here. (#12345, #12346, #12347)
`

	parsed, err := ParseChangelog(changelog)
	require.NoError(t, err)

	require.Len(t, parsed.BreakingChanges, 1)
	assert.Equal(t, []string{"#12345", "#12346", "#12347"}, parsed.BreakingChanges[0].IssueRefs)
}

func TestParseChangelog_MultilineSubtext(t *testing.T) {
	changelog := `### Breaking changes

- ` + "`component`" + `: Main description. (#12345)
  First line of subtext.
  Second line of subtext.
`

	parsed, err := ParseChangelog(changelog)
	require.NoError(t, err)

	require.Len(t, parsed.BreakingChanges, 1)
	assert.Equal(t, "Main description.", parsed.BreakingChanges[0].Note)
	assert.Equal(t, "First line of subtext.\nSecond line of subtext.", parsed.BreakingChanges[0].SubText)
}

func TestParseChangelog_ComponentFormats(t *testing.T) {
	changelog := `### Breaking changes

- ` + "`receiver/sapm`" + `: With slash format. (#1)
- ` + "`transformprocessor`" + `: Suffix format. (#2)
- ` + "`exporter/kafkaexporter`" + `: Both slash and suffix. (#3)
`

	parsed, err := ParseChangelog(changelog)
	require.NoError(t, err)

	require.Len(t, parsed.BreakingChanges, 3)
	assert.Equal(t, "receiver/sapm", parsed.BreakingChanges[0].Component)
	assert.Equal(t, "transformprocessor", parsed.BreakingChanges[1].Component)
	assert.Equal(t, "exporter/kafkaexporter", parsed.BreakingChanges[2].Component)
}

func TestParseChangelog_EmptyChangelog(t *testing.T) {
	parsed, err := ParseChangelog("")
	require.NoError(t, err)

	assert.Empty(t, parsed.BreakingChanges)
	assert.Empty(t, parsed.Deprecations)
	assert.Empty(t, parsed.NewComponents)
	assert.Empty(t, parsed.Enhancements)
	assert.Empty(t, parsed.BugFixes)
}

func TestParseChangelog_RealChangelog(t *testing.T) {
	// Test with actual changelog format from 0.135.0
	changelog := `## v0.135.0

### 🛑 Breaking changes 🛑

- ` + "`apachereceiver`" + `: Add number of connections per async state metrics. (#41886)
- ` + "`githubreceiver`" + `: Update semantic conventions from v1.27.0 to v1.37.0 with standardized VCS and CICD attributes (#42378)
  - Resource attributes changed: ` + "`organization.name`" + ` -> ` + "`vcs.owner.name`" + `, ` + "`vcs.vendor.name`" + ` -> ` + "`vcs.provider.name`" + `
  - Trace attributes now use standardized VCS naming: ` + "`vcs.ref.head.type`" + ` -> ` + "`vcs.ref.type`" + `

- ` + "`k8sattributesprocessor`" + `: Introduce allowLabelsAnnotationsSingular feature gate (#39774)
- ` + "`receiver/sapm`" + `: The SAPM Receiver component has been removed from the repo (#41411)

### 💡 Enhancements 💡

- ` + "`transformprocessor`" + `: Add support for merging histogram buckets. (#40280)
  The transformprocessor now supports merging histogram buckets using the merge_histogram_buckets function.

### 🧰 Bug fixes 🧰

- ` + "`libhoneyreceiver`" + `: Properly handle compressed payloads (#42279)
  Compression issues now return a 400 status rather than panic.
`

	parsed, err := ParseChangelog(changelog)
	require.NoError(t, err)

	// Verify breaking changes
	require.Len(t, parsed.BreakingChanges, 4)
	assert.Equal(t, "apachereceiver", parsed.BreakingChanges[0].Component)
	assert.Equal(t, "githubreceiver", parsed.BreakingChanges[1].Component)
	assert.Equal(t, "k8sattributesprocessor", parsed.BreakingChanges[2].Component)
	assert.Equal(t, "receiver/sapm", parsed.BreakingChanges[3].Component)

	// Verify enhancements
	require.Len(t, parsed.Enhancements, 1)
	assert.Equal(t, "transformprocessor", parsed.Enhancements[0].Component)
	assert.Contains(t, parsed.Enhancements[0].SubText, "merge_histogram_buckets")

	// Verify bug fixes
	require.Len(t, parsed.BugFixes, 1)
	assert.Equal(t, "libhoneyreceiver", parsed.BugFixes[0].Component)
}
