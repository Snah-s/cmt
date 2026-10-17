package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Snah-s/cmt/internal/commit"
)

// customScope marks the "Custom scope…" option; the NUL byte can never be a valid scope.
const customScope = "\x00custom"

// scopeContainers hold one module per child folder, so the child names the scope.
var scopeContainers = []string{"internal", "cmd", "pkg", "src", "packages", "apps", "libs", "services"}

// scopeFromPath names the module of a repository path; root files and dot folders suggest nothing.
// shortcut: a fixed folder rule; replace it with smarter detection (IMPROVEMENTS 5) when it misleads.
func scopeFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	name := parts[0]
	if len(parts) > 2 && slices.Contains(scopeContainers, name) {
		name = parts[1]
	}
	name = strings.ToLower(name)
	if strings.HasPrefix(name, ".") || commit.ValidateScope(name) != nil {
		return ""
	}
	return name
}

type scopeOption struct{ name, note string }

// scopeChoices orders the candidates: the received scope, staged modules by file count, saved
// custom scopes, then main folders. A project list is strict and replaces detection.
func (m *commitForm) scopeChoices() []scopeOption {
	var choices []scopeOption
	add := func(name, note string) {
		if name == "" || name == customScope || commit.ValidateScope(name) != nil ||
			slices.ContainsFunc(choices, func(c scopeOption) bool { return c.name == name }) {
			return
		}
		choices = append(choices, scopeOption{name, note})
	}
	add(m.draft.Scope, "current")
	if m.options.Scopes != nil {
		for _, scope := range m.options.Scopes {
			add(scope, "")
		}
		return choices
	}
	counts := make(map[string]int)
	var staged []string
	for _, path := range m.options.StagedPaths {
		if scope := scopeFromPath(path); scope != "" {
			if counts[scope] == 0 {
				staged = append(staged, scope)
			}
			counts[scope]++
		}
	}
	slices.SortStableFunc(staged, func(a, b string) int { return counts[b] - counts[a] })
	for _, scope := range staged {
		note := "staged · 1 file"
		if counts[scope] > 1 {
			note = fmt.Sprintf("staged · %d files", counts[scope])
		}
		add(scope, note)
	}
	for _, scope := range m.options.CustomScopes {
		add(scope, "saved")
	}
	var folders []string
	for _, path := range m.options.TrackedPaths {
		if scope := scopeFromPath(path); scope != "" && !slices.Contains(folders, scope) {
			folders = append(folders, scope)
		}
	}
	slices.Sort(folders)
	for _, scope := range folders {
		add(scope, "")
	}
	return choices
}
