package piagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	StatusImported = "imported"
	StatusEmpty    = "empty"
	StatusNotFound = "not_found"
)

// Package is the normalized metadata pi exposes through its package system.
// SourcePath is kept for the importer/DB projection and is not required in the
// browser response.
type Package struct {
	ID            string
	Spec          string
	Type          string
	Name          string
	Version       string
	Description   string
	Homepage      string
	HasExtensions bool
	HasSkills     bool
	HasPrompts    bool
	HasThemes     bool
	Enabled       bool
	SourcePath    string
	Manifest      string
}

type ImportResult struct {
	OK         bool      `json:"ok"`
	Count      int       `json:"count"`
	Discovered int       `json:"discovered"`
	Skipped    int       `json:"skipped"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	Warnings   []string  `json:"warnings,omitempty"`
	Packages   []Package `json:"-"`
}

type sourceInfo struct {
	spec           string
	enabled        bool
	baseDir        string
	explicitFilter bool
	filters        map[string][]string
}

type settingsState struct {
	found   bool
	sources map[string]sourceInfo
	warns   []string
}

// AgentRoot follows Pi's current global agent directory contract. The explicit
// test override keeps discovery hermetic without changing production defaults.
func AgentRoot() string {
	if p := strings.TrimSpace(os.Getenv("PI_AGENT_ROOT")); p != "" {
		return filepath.Clean(p)
	}
	if p := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); p != "" {
		return filepath.Clean(p)
	}
	if p := strings.TrimSpace(os.Getenv("PI_AGENT_SETTINGS")); p != "" {
		return filepath.Dir(filepath.Clean(p))
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("/tmp", ".pi", "agent")
	}
	return filepath.Join(home, ".pi", "agent")
}

func settingsPath(root string) string {
	if p := strings.TrimSpace(os.Getenv("PI_AGENT_SETTINGS")); p != "" {
		return filepath.Clean(p)
	}
	return filepath.Join(root, "settings.json")
}

// DiscoverPackages scans configured Pi sources and the managed/auto-discovered
// resource roots. It never writes files or the pi-switch database.
func DiscoverPackages() (ImportResult, error) {
	root := AgentRoot()
	state, err := loadSettings(settingsPath(root))
	if err != nil {
		return ImportResult{}, err
	}
	rootExists := directoryExists(root)

	candidatePaths := map[string]bool{}
	for canonical, source := range state.sources {
		if path, ok := localSourcePath(source.spec, source.baseDir); ok {
			candidatePaths[filepath.Clean(path)] = true
			continue
		}
		_ = canonical
	}
	for _, base := range managedRoots(root) {
		for _, manifest := range findManifests(base) {
			candidatePaths[filepath.Dir(manifest)] = true
		}
		for _, candidate := range findPotentialPackageRoots(base) {
			candidatePaths[candidate] = true
		}
	}

	paths := make([]string, 0, len(candidatePaths))
	for path := range candidatePaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := ImportResult{OK: true, Status: StatusImported, Warnings: append([]string{}, state.warns...)}
	byID := map[string]Package{}
	for _, path := range paths {
		pkg, warning, ok := inspectPackage(path, state.sources)
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
		if !ok {
			result.Skipped++
			continue
		}
		result.Discovered++
		if previous, exists := byID[pkg.ID]; exists {
			byID[pkg.ID] = mergePackage(previous, pkg)
			continue
		}
		byID[pkg.ID] = pkg
	}

	result.Packages = make([]Package, 0, len(byID))
	for _, pkg := range byID {
		result.Packages = append(result.Packages, pkg)
	}
	sort.Slice(result.Packages, func(i, j int) bool { return result.Packages[i].ID < result.Packages[j].ID })
	result.Count = len(result.Packages)
	if result.Count == 0 {
		if !rootExists && !state.found {
			result.OK = false
			result.Status = StatusNotFound
			result.Message = "Pi Agent package roots were not found"
			return result, nil
		}
		result.Status = StatusEmpty
		result.Message = "No valid Pi packages were discovered"
		return result, nil
	}
	result.Message = fmt.Sprintf("Imported %d Pi package(s)", result.Count)
	return result, nil
}

func loadSettings(path string) (settingsState, error) {
	state := settingsState{sources: map[string]sourceInfo{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read Pi Agent settings %s: %w", path, err)
	}
	state.found = true
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return state, fmt.Errorf("invalid Pi Agent settings %s: %w", path, err)
	}
	items, ok := raw["packages"]
	if !ok {
		return state, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(items, &entries); err != nil {
		return state, fmt.Errorf("invalid Pi Agent settings packages: %w", err)
	}
	baseDir := filepath.Dir(path)
	for index, entry := range entries {
		var source string
		info := sourceInfo{enabled: true, baseDir: baseDir}
		if err := json.Unmarshal(entry, &source); err == nil {
			info.spec = strings.TrimSpace(source)
		} else {
			var object struct {
				Source     string   `json:"source"`
				Autoload   *bool    `json:"autoload"`
				Extensions []string `json:"extensions"`
				Skills     []string `json:"skills"`
				Prompts    []string `json:"prompts"`
				Themes     []string `json:"themes"`
			}
			if err := json.Unmarshal(entry, &object); err != nil {
				state.warns = append(state.warns, fmt.Sprintf("settings packages[%d] is not a string or object", index))
				continue
			}
			info.spec = strings.TrimSpace(object.Source)
			info.filters = map[string][]string{}
			var rawObject map[string]json.RawMessage
			_ = json.Unmarshal(entry, &rawObject)
			for _, key := range []string{"extensions", "skills", "prompts", "themes"} {
				if rawValue, exists := rawObject[key]; exists {
					var patterns []string
					if json.Unmarshal(rawValue, &patterns) == nil {
						info.filters[key] = patterns
						info.explicitFilter = true
					}
				}
			}
			if object.Autoload != nil {
				info.enabled = *object.Autoload
			}
		}
		if info.spec == "" {
			state.warns = append(state.warns, fmt.Sprintf("settings packages[%d] has no source", index))
			continue
		}
		state.sources[canonicalSource(info.spec, baseDir)] = info
	}
	return state, nil
}

func managedRoots(root string) []string {
	roots := []string{
		filepath.Join(root, "npm", "node_modules"),
		filepath.Join(root, "git"),
		filepath.Join(root, "extensions"),
		filepath.Join(root, "skills"),
		filepath.Join(root, "prompts"),
		filepath.Join(root, "themes"),
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, filepath.Join(home, ".agents", "skills"))
	}
	return roots
}

func findManifests(base string) []string {
	if !directoryExists(base) {
		return nil
	}
	var manifests []string
	_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != base && (entry.Name() == "node_modules" || entry.Name() == ".git" || strings.HasPrefix(entry.Name(), ".")) {
				return fs.SkipDir
			}
			rel, _ := filepath.Rel(base, path)
			if rel != "." && strings.Count(rel, string(os.PathSeparator)) > 6 {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Name() == "package.json" {
			manifests = append(manifests, path)
		}
		return nil
	})
	sort.Strings(manifests)
	return manifests
}

func findPotentialPackageRoots(base string) []string {
	if !strings.Contains(filepath.ToSlash(base), "/node_modules") || !directoryExists(base) {
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var roots []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(base, entry.Name())
		if strings.HasPrefix(entry.Name(), "@") {
			children, _ := os.ReadDir(path)
			for _, child := range children {
				if child.IsDir() && !strings.HasPrefix(child.Name(), ".") {
					roots = append(roots, filepath.Join(path, child.Name()))
				}
			}
			continue
		}
		roots = append(roots, path)
	}
	return roots
}

func inspectPackage(path string, sources map[string]sourceInfo) (Package, string, bool) {
	manifestPath := filepath.Join(path, "package.json")
	data, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return Package{}, fmt.Sprintf("Pi resource root %s has no package.json", path), false
	}
	if err != nil {
		return Package{}, fmt.Sprintf("cannot read Pi package manifest %s: %v", manifestPath, err), false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Package{}, fmt.Sprintf("invalid Pi package manifest %s: %v", manifestPath, err), false
	}
	var meta struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
	}
	_ = json.Unmarshal(data, &meta)

	piRaw, hasPi := raw["pi"]
	caps, validPi := manifestCapabilities(path, piRaw, hasPi)
	if !validPi {
		return Package{}, fmt.Sprintf("Pi package %s has no valid resource manifest", path), false
	}
	if strings.TrimSpace(meta.Name) == "" {
		meta.Name = filepath.Base(path)
	}

	canonical, info, configured := packageSourceIdentity(path, meta.Name, sources)
	spec := "local:" + path
	enabled := true
	if configured {
		spec = info.spec
		enabled = info.enabled && filtersEnableAny(info)
	}
	pkgType, _ := parseSpec(spec)
	manifest := map[string]interface{}{}
	_ = json.Unmarshal(data, &manifest)
	normalized, _ := json.Marshal(manifest)
	return Package{
		ID:            canonical,
		Spec:          spec,
		Type:          pkgType,
		Name:          meta.Name,
		Version:       meta.Version,
		Description:   meta.Description,
		Homepage:      meta.Homepage,
		HasExtensions: caps.extensions,
		HasSkills:     caps.skills,
		HasPrompts:    caps.prompts,
		HasThemes:     caps.themes,
		Enabled:       enabled,
		SourcePath:    path,
		Manifest:      string(normalized),
	}, "", true
}

func packageSourceIdentity(path, name string, sources map[string]sourceInfo) (string, sourceInfo, bool) {
	local := canonicalSource("local:"+path, path)
	if info, ok := sources[local]; ok {
		return local, info, true
	}
	for canonical, info := range sources {
		if strings.HasPrefix(canonical, "npm:") && strings.TrimPrefix(canonical, "npm:") == name {
			return canonical, info, true
		}
		if strings.HasPrefix(canonical, "git:") && (filepath.Base(path) == filepath.Base(strings.TrimPrefix(canonical, "git:")) || strings.Contains(path, filepath.FromSlash(strings.TrimPrefix(canonical, "git:")))) {
			return canonical, info, true
		}
	}
	clean := filepath.Clean(path)
	marker := string(filepath.Separator) + "npm" + string(filepath.Separator) + "node_modules" + string(filepath.Separator)
	if idx := strings.Index(clean, marker); idx >= 0 {
		return "npm:" + name, sourceInfo{}, false
	}
	if idx := strings.Index(clean, string(filepath.Separator)+"git"+string(filepath.Separator)); idx >= 0 {
		return "git:" + filepath.ToSlash(clean[idx+len(string(filepath.Separator)+"git"+string(filepath.Separator)):]), sourceInfo{}, false
	}
	return local, sourceInfo{}, false
}

func filtersEnableAny(info sourceInfo) bool {
	if !info.explicitFilter {
		return true
	}
	for _, patterns := range info.filters {
		if len(patterns) == 0 {
			continue
		}
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" && !strings.HasPrefix(pattern, "-") && !strings.HasPrefix(pattern, "!") {
				return true
			}
		}
	}
	return false
}

type capabilities struct {
	extensions bool
	skills     bool
	prompts    bool
	themes     bool
}

func manifestCapabilities(root string, raw json.RawMessage, hasPi bool) (capabilities, bool) {
	if !hasPi {
		result := capabilities{
			extensions: directoryHasEntries(filepath.Join(root, "extensions")),
			skills:     directoryHasEntries(filepath.Join(root, "skills")),
			prompts:    directoryHasEntries(filepath.Join(root, "prompts")),
			themes:     directoryHasEntries(filepath.Join(root, "themes")),
		}
		return result, result.extensions || result.skills || result.prompts || result.themes
	}
	var pi map[string]json.RawMessage
	if json.Unmarshal(raw, &pi) != nil {
		return capabilities{}, false
	}
	var result capabilities
	for key, target := range map[string]*bool{
		"extensions": &result.extensions,
		"skills":     &result.skills,
		"prompts":    &result.prompts,
		"themes":     &result.themes,
	} {
		value, exists := pi[key]
		if !exists {
			continue
		}
		var entries []string
		if json.Unmarshal(value, &entries) != nil {
			continue
		}
		*target = len(entries) > 0
	}
	return result, true
}

func directoryHasEntries(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

func mergePackage(left, right Package) Package {
	left.HasExtensions = left.HasExtensions || right.HasExtensions
	left.HasSkills = left.HasSkills || right.HasSkills
	left.HasPrompts = left.HasPrompts || right.HasPrompts
	left.HasThemes = left.HasThemes || right.HasThemes
	left.Enabled = left.Enabled && right.Enabled
	if left.Version == "" {
		left.Version = right.Version
	}
	if left.Description == "" {
		left.Description = right.Description
	}
	if left.Homepage == "" {
		left.Homepage = right.Homepage
	}
	return left
}

func localSourcePath(spec, baseDir string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "local:") {
		spec = strings.TrimPrefix(spec, "local:")
	}
	if spec == "" || strings.HasPrefix(spec, "npm:") || strings.HasPrefix(spec, "git:") || strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://") || strings.HasPrefix(spec, "ssh://") {
		return "", false
	}
	if !filepath.IsAbs(spec) {
		spec = filepath.Join(baseDir, spec)
	}
	return filepath.Clean(spec), true
}

func canonicalSource(spec, baseDir string) string {
	spec = strings.TrimSpace(spec)
	if path, ok := localSourcePath(spec, baseDir); ok {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
		return "local:" + filepath.Clean(path)
	}
	if strings.HasPrefix(spec, "npm:") {
		name := strings.TrimPrefix(spec, "npm:")
		if at := strings.LastIndex(name, "@"); at > 0 {
			name = name[:at]
		}
		return "npm:" + name
	}
	if strings.HasPrefix(spec, "git:") {
		name := strings.TrimPrefix(spec, "git:")
		if at := strings.LastIndex(name, "@"); at > 0 {
			name = name[:at]
		}
		return "git:" + strings.TrimSuffix(name, ".git")
	}
	return "local:" + strings.TrimSpace(spec)
}

func parseSpec(spec string) (string, string) {
	spec = strings.TrimSpace(spec)
	if strings.Contains(spec, ":") {
		parts := strings.SplitN(spec, ":", 2)
		return parts[0], parts[1]
	}
	if strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, ".") {
		return "local", spec
	}
	return "npm", spec
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
