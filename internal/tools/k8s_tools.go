package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/open-telemetry/opentelemetry-operator/apis/v1beta1"
	"github.com/niwoerner/opentelemetry-mcp-server/internal/k8s"
	"github.com/niwoerner/opentelemetry-mcp-server/modules/collectorschema"
)

func getK8sCollectorsDiscoveryTool() Tool {
	tool := mcp.NewTool("opentelemetry-k8s-collectors",
		mcp.WithDescription("Discover OpenTelemetryCollector CRs in the current Kubernetes cluster and extract their configs"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("namespace",
			mcp.Description("Namespace to search (empty = all namespaces)"),
		),
		mcp.WithString("name",
			mcp.Description("Specific collector name (requires namespace)"),
		),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := k8s.NewClient()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to connect to cluster: %v", err)), nil
		}

		namespace := request.GetString("namespace", "")
		name := request.GetString("name", "")

		if name != "" {
			if namespace == "" {
				return mcp.NewToolResultError("namespace is required when name is specified"), nil
			}
			collector, err := client.GetCollector(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get collector: %v", err)), nil
			}
			result, _ := json.MarshalIndent(collector, "", "  ")
			return mcp.NewToolResultText(string(result)), nil
		}

		collectors, err := client.ListCollectors(ctx, namespace)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to list collectors: %v", err)), nil
		}

		result, _ := json.MarshalIndent(collectors, "", "  ")
		return mcp.NewToolResultText(string(result)), nil
	}

	return Tool{Tool: tool, Handler: handler}
}

// UpgradeStep represents deprecations and changelog changes for a specific version
type UpgradeStep struct {
	Version         string                            `json:"version"`
	Deprecations    []ComponentDeprecation            `json:"deprecations,omitempty"`
	BreakingChanges []ChangelogChange                 `json:"breaking_changes,omitempty"`
	BugFixes        []ChangelogChange                 `json:"bug_fixes,omitempty"`
}

// ComponentDeprecation contains deprecated fields for a component
type ComponentDeprecation struct {
	Kind             string                            `json:"kind"`
	Name             string                            `json:"name"`
	DeprecatedFields []collectorschema.DeprecatedField `json:"deprecated_fields"`
}

// ChangelogChange represents a change from the changelog that affects a component
type ChangelogChange struct {
	Component  string                 `json:"component"`
	Note       string                 `json:"note"`
	IssueRefs  []string               `json:"issue_refs,omitempty"`
	UserConfig map[string]any `json:"user_config,omitempty"`
}

// CollectorUpgradeReport represents the upgrade path for a single collector
type CollectorUpgradeReport struct {
	Name           string        `json:"name"`
	Namespace      string        `json:"namespace"`
	CurrentVersion string        `json:"current_version"`
	TargetVersion  string        `json:"target_version"`
	Warning        string        `json:"warning,omitempty"`
	UpgradeSteps   []UpgradeStep `json:"upgrade_steps,omitempty"`
}

// UpgradePathResult is the full result returned by the tool
type UpgradePathResult struct {
	Collectors []CollectorUpgradeReport `json:"collectors"`
}

func getK8sCollectorUpgradePathTool(schemaManager *collectorschema.SchemaManager, latestCollectorVersion string) Tool {
	tool := mcp.NewTool("opentelemetry-k8s-collector-upgrade-path",
		mcp.WithDescription("Generate upgrade reports for OpenTelemetryCollector CRs showing deprecations and breaking changes for each version from current to target"),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("namespace",
			mcp.Description("Namespace to search (empty = all namespaces)"),
		),
		mcp.WithString("name",
			mcp.Description("Specific collector name (requires namespace)"),
		),
		mcp.WithString("target_version",
			mcp.Description("Target version (default: latest supported version)"),
		),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := k8s.NewClient()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to connect to cluster: %v", err)), nil
		}

		namespace := request.GetString("namespace", "")
		name := request.GetString("name", "")
		targetVersion := request.GetString("target_version", latestCollectorVersion)

		// Get all supported versions
		allVersions, err := schemaManager.GetAllVersions()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get supported versions: %v", err)), nil
		}

		var collectors []k8s.CollectorCustomResource
		if name != "" {
			if namespace == "" {
				return mcp.NewToolResultError("namespace is required when name is specified"), nil
			}
			collector, err := client.GetCollector(ctx, namespace, name)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get collector: %v", err)), nil
			}
			collectors = []k8s.CollectorCustomResource{*collector}
		} else {
			collectors, err = client.ListCollectors(ctx, namespace)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list collectors: %v", err)), nil
			}
		}

		result := UpgradePathResult{
			Collectors: make([]CollectorUpgradeReport, 0, len(collectors)),
		}

		for _, collector := range collectors {
			report := buildUpgradeReport(collector, targetVersion, allVersions, schemaManager)
			result.Collectors = append(result.Collectors, report)
		}

		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil
	}

	return Tool{Tool: tool, Handler: handler}
}

func buildUpgradeReport(collector k8s.CollectorCustomResource, targetVersion string, allVersions []string, schemaManager *collectorschema.SchemaManager) CollectorUpgradeReport {
	report := CollectorUpgradeReport{
		Name:           collector.Name,
		Namespace:      collector.Namespace,
		CurrentVersion: collector.Version,
		TargetVersion:  targetVersion,
	}

	if collector.Version == "" {
		report.Warning = "collector has no version in status, skipping upgrade analysis"
		return report
	}

	// Filter versions between current and target
	upgradeVersions := filterVersionsBetween(collector.Version, targetVersion, allVersions)
	if len(upgradeVersions) == 0 {
		return report
	}

	// Extract component names from the collector config
	components := extractComponents(collector.Config)

	// Build upgrade steps for each version
	for _, ver := range upgradeVersions {
		step := buildUpgradeStep(ver, components, schemaManager)
		if len(step.Deprecations) > 0 || len(step.BreakingChanges) > 0 || len(step.BugFixes) > 0 {
			report.UpgradeSteps = append(report.UpgradeSteps, step)
		}
	}

	return report
}

// ComponentInfo holds kind and config for a component
type ComponentInfo struct {
	Kind   string
	Name   string
	Config map[string]any
}

func extractComponents(config v1beta1.Config) []ComponentInfo {
	var components []ComponentInfo

	componentMaps := []struct {
		kind string
		obj  map[string]any
	}{
		{"receiver", config.Receivers.Object},
		{"exporter", config.Exporters.Object},
	}
	if config.Processors != nil {
		componentMaps = append(componentMaps, struct {
			kind string
			obj  map[string]any
		}{"processor", config.Processors.Object})
	}
	if config.Connectors != nil {
		componentMaps = append(componentMaps, struct {
			kind string
			obj  map[string]any
		}{"connector", config.Connectors.Object})
	}
	if config.Extensions != nil {
		componentMaps = append(componentMaps, struct {
			kind string
			obj  map[string]any
		}{"extension", config.Extensions.Object})
	}

	for _, cm := range componentMaps {
		for name, cfg := range cm.obj {
			cfgMap, _ := cfg.(map[string]any)
			components = append(components, ComponentInfo{
				Kind:   cm.kind,
				Name:   extractBaseName(name),
				Config: cfgMap,
			})
		}
	}

	return components
}

// extractBaseName extracts the base component name from "component/suffix" format
func extractBaseName(name string) string {
	if idx := strings.Index(name, "/"); idx != -1 {
		return name[:idx]
	}
	return name
}

func buildUpgradeStep(ver string, components []ComponentInfo, schemaManager *collectorschema.SchemaManager) UpgradeStep {
	step := UpgradeStep{Version: ver}

	// Get deprecations from schema
	for _, comp := range components {
		deprecatedFields, err := schemaManager.GetDeprecatedFields(collectorschema.ComponentType(comp.Kind), comp.Name, ver)
		if err == nil && len(deprecatedFields) > 0 {
			step.Deprecations = append(step.Deprecations, ComponentDeprecation{
				Kind:             comp.Kind,
				Name:             comp.Name,
				DeprecatedFields: deprecatedFields,
			})
		}
	}

	// Get breaking changes and bug fixes from changelog
	parsed, err := schemaManager.GetParsedChangelog(ver)
	if err == nil {
		// Add matching breaking changes
		for _, entry := range parsed.BreakingChanges {
			match := componentMatchesChangelog(components, entry.Component)
			if match != nil {
				step.BreakingChanges = append(step.BreakingChanges, ChangelogChange{
					Component:  entry.Component,
					Note:       entry.Note,
					IssueRefs:  entry.IssueRefs,
					UserConfig: match.Config,
				})
			}
		}

		// Add matching bug fixes
		for _, entry := range parsed.BugFixes {
			match := componentMatchesChangelog(components, entry.Component)
			if match != nil {
				step.BugFixes = append(step.BugFixes, ChangelogChange{
					Component:  entry.Component,
					Note:       entry.Note,
					IssueRefs:  entry.IssueRefs,
					UserConfig: match.Config,
				})
			}
		}
	}

	return step
}

// componentMatchesChangelog checks if any user component matches the changelog component
// using simple contains matching. Returns the matching component or nil.
func componentMatchesChangelog(userComponents []ComponentInfo, changelogComponent string) *ComponentInfo {
	changelogLower := strings.ToLower(changelogComponent)
	for _, comp := range userComponents {
		if strings.Contains(changelogLower, strings.ToLower(comp.Name)) {
			return &comp
		}
	}
	return nil
}

func filterVersionsBetween(current, target string, allVersions []string) []string {
	currentV, err := version.NewVersion(current)
	if err != nil {
		return nil
	}
	targetV, err := version.NewVersion(target)
	if err != nil {
		return nil
	}

	var result []string
	for _, v := range allVersions {
		ver, err := version.NewVersion(v)
		if err != nil {
			continue
		}
		// Include versions where current < v <= target
		if ver.GreaterThan(currentV) && (ver.LessThan(targetV) || ver.Equal(targetV)) {
			result = append(result, v)
		}
	}

	// Sort versions (allVersions is already sorted by directory listing, but let's be safe)
	return result
}
