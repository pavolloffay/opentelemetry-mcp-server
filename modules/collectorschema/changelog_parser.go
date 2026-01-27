package collectorschema

import (
	"regexp"
	"strings"
)

// ChangelogEntry represents a single entry from any changelog section
type ChangelogEntry struct {
	Component string   // Raw component name (e.g., "transformprocessor", "receiver/sapm")
	Note      string   // Main description
	IssueRefs []string // e.g., ["#41886", "#41887"]
	SubText   string   // Optional indented additional details
}

// ParsedChangelog contains all parsed sections from a changelog
// Source format: https://github.com/open-telemetry/opentelemetry-collector/blob/main/.chloggen/summary.tmpl
type ParsedChangelog struct {
	BreakingChanges []ChangelogEntry
	Deprecations    []ChangelogEntry
	NewComponents   []ChangelogEntry
	Enhancements    []ChangelogEntry
	BugFixes        []ChangelogEntry
}

// Section header patterns (case-insensitive, ignores emojis)
var sectionPatterns = map[string]*regexp.Regexp{
	"breaking":     regexp.MustCompile(`(?i)^###.*breaking\s+changes`),
	"deprecations": regexp.MustCompile(`(?i)^###.*deprecations`),
	"new":          regexp.MustCompile(`(?i)^###.*new\s+components`),
	"enhancements": regexp.MustCompile(`(?i)^###.*enhancements`),
	"bugfixes":     regexp.MustCompile(`(?i)^###.*bug\s*fixes`),
}

// Patterns for parsing entries
var (
	// Matches component name in backticks: `componentname`
	componentPattern = regexp.MustCompile("`([^`]+)`")
	// Matches issue references: (#12345) or (#12345, #12346)
	issuePattern = regexp.MustCompile(`\(#(\d+(?:,\s*#\d+)*)\)`)
	// Matches individual issue numbers
	issueNumberPattern = regexp.MustCompile(`#(\d+)`)
	// Matches a new entry line starting with "- "
	entryLinePattern = regexp.MustCompile(`^-\s+`)
	// Matches a section header
	sectionHeaderPattern = regexp.MustCompile(`^###\s+`)
)

// saveEntry finalizes and appends the entry to the slice
func saveEntry(entry *ChangelogEntry, entries *[]ChangelogEntry) {
	if entry == nil || entries == nil {
		return
	}
	entry.Note = strings.TrimSpace(entry.Note)
	entry.SubText = strings.TrimSpace(entry.SubText)
	*entries = append(*entries, *entry)
}

// ParseChangelog parses all sections from changelog markdown
func ParseChangelog(changelog string) (*ParsedChangelog, error) {
	parsed := &ParsedChangelog{}
	lines := strings.Split(changelog, "\n")

	var currentSection string
	var currentEntries *[]ChangelogEntry
	var currentEntry *ChangelogEntry

	for _, line := range lines {
		// Check if this is a section header
		if sectionHeaderPattern.MatchString(line) {
			saveEntry(currentEntry, currentEntries)
			currentEntry = nil
			currentSection = ""
			currentEntries = nil

			switch {
			case sectionPatterns["breaking"].MatchString(line):
				currentSection = "breaking"
				currentEntries = &parsed.BreakingChanges
			case sectionPatterns["deprecations"].MatchString(line):
				currentSection = "deprecations"
				currentEntries = &parsed.Deprecations
			case sectionPatterns["new"].MatchString(line):
				currentSection = "new"
				currentEntries = &parsed.NewComponents
			case sectionPatterns["enhancements"].MatchString(line):
				currentSection = "enhancements"
				currentEntries = &parsed.Enhancements
			case sectionPatterns["bugfixes"].MatchString(line):
				currentSection = "bugfixes"
				currentEntries = &parsed.BugFixes
			}
			continue
		}

		// Skip if not in a known section
		if currentSection == "" || currentEntries == nil {
			continue
		}

		// Check if this is a new entry
		if entryLinePattern.MatchString(line) {
			saveEntry(currentEntry, currentEntries)
			currentEntry = parseEntryLine(line)
			continue
		}

		// Check if this is subtext (indented continuation)
		if currentEntry != nil && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			if currentEntry.SubText != "" {
				currentEntry.SubText += "\n"
			}
			currentEntry.SubText += strings.TrimSpace(line)
		}
	}

	saveEntry(currentEntry, currentEntries)
	return parsed, nil
}

// parseEntryLine parses a single entry line like:
// - `component`: Description here. (#12345)
func parseEntryLine(line string) *ChangelogEntry {
	entry := &ChangelogEntry{}

	// Remove leading "- "
	line = entryLinePattern.ReplaceAllString(line, "")

	// Extract component name
	if match := componentPattern.FindStringSubmatch(line); len(match) > 1 {
		entry.Component = match[1]
	}

	// Extract issue references
	if match := issuePattern.FindString(line); match != "" {
		issueMatches := issueNumberPattern.FindAllStringSubmatch(match, -1)
		for _, m := range issueMatches {
			if len(m) > 1 {
				entry.IssueRefs = append(entry.IssueRefs, "#"+m[1])
			}
		}
	}

	// Extract note (text after component, before issue refs)
	note := line

	// Remove component part
	if idx := strings.Index(note, "`:"); idx != -1 {
		note = note[idx+2:]
	} else if idx := strings.Index(note, "`"); idx != -1 {
		// Handle case where there's no colon after component
		endIdx := strings.Index(note[idx+1:], "`")
		if endIdx != -1 {
			note = note[idx+endIdx+2:]
		}
	}

	// Remove issue refs from note
	note = issuePattern.ReplaceAllString(note, "")
	entry.Note = strings.TrimSpace(note)

	return entry
}
