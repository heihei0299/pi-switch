package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type violation struct {
	File       string
	Line       int
	ImportPath string
	Reason     string
}

func (v violation) String() string {
	return fmt.Sprintf("%s:%d: import %q: %s", v.File, v.Line, v.ImportPath, v.Reason)
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	module, err := modulePath(root)
	if err != nil {
		fatal(err)
	}
	stdlib, err := standardPackages(root)
	if err != nil {
		fatal(err)
	}
	violations, err := check(root, module, stdlib)
	if err != nil {
		fatal(err)
	}
	if len(violations) != 0 {
		for _, item := range violations {
			fmt.Fprintln(os.Stderr, item)
		}
		os.Exit(1)
	}
	fmt.Println("architecture dependencies: clean")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}

func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "module" {
			continue
		}
		module := fields[1]
		if strings.HasPrefix(module, `"`) {
			module, err = strconv.Unquote(module)
		}
		if err != nil || module == "" {
			return "", fmt.Errorf("invalid module directive in %s/go.mod", root)
		}
		return module, nil
	}
	return "", fmt.Errorf("module directive not found in %s/go.mod", root)
}

func standardPackages(root string) (map[string]bool, error) {
	cmd := exec.Command("go", "list", "std")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list standard library packages: %w", err)
	}
	packages := make(map[string]bool)
	for _, name := range strings.Fields(string(output)) {
		packages[name] = true
	}
	return packages, nil
}

func check(root, module string, stdlib map[string]bool) ([]violation, error) {
	var violations []violation
	fset := token.NewFileSet()
	projectDirs := []string{"cmd", "internal", "legacy", "webui"}
	for _, relDir := range projectDirs {
		dir := filepath.Join(root, relDir)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != dir && strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relSlash := filepath.ToSlash(rel)
			if strings.HasPrefix(relSlash, "internal/adapters/") &&
				!strings.HasPrefix(relSlash, "internal/adapters/inbound/") &&
				!strings.HasPrefix(relSlash, "internal/adapters/outbound/") {
				violations = append(violations, violation{rel, 1, "<package>", "adapters code must live under inbound or outbound"})
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				position := fset.Position(spec.Pos())
				if under(importPath, module+"/desktop") {
					violations = append(violations, violation{rel, position.Line, importPath, "root module must not import the separate desktop module"})
					continue
				}
				if reason := forbiddenImport(filepath.ToSlash(rel), importPath, module, stdlib); reason != "" {
					violations = append(violations, violation{rel, position.Line, importPath, reason})
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Root-level Go files belong to the root module too; desktop is deliberately
	// excluded because it has its own go.mod and composes the shared root module.
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, err
			}
			if under(importPath, module+"/desktop") {
				position := fset.Position(spec.Pos())
				violations = append(violations, violation{entry.Name(), position.Line, importPath, "root module must not import the separate desktop module"})
			}
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].ImportPath < violations[j].ImportPath
	})
	return violations, nil
}

func forbiddenImport(file, importPath, module string, stdlib map[string]bool) string {
	rel := filepath.ToSlash(file)
	layer := ""
	switch {
	case strings.HasPrefix(rel, "internal/domain/"):
		layer = "domain"
	case strings.HasPrefix(rel, "internal/usecase/"):
		layer = "usecase"
	case strings.HasPrefix(rel, "internal/adapters/inbound/"):
		layer = "inbound"
	case strings.HasPrefix(rel, "internal/adapters/outbound/"):
		layer = "outbound"
	case strings.HasPrefix(rel, "internal/bootstrap/"):
		layer = "bootstrap"
	default:
		return ""
	}

	domain := module + "/internal/domain"
	usecase := module + "/internal/usecase"
	inbound := module + "/internal/adapters/inbound"
	outbound := module + "/internal/adapters/outbound"
	allowed := false
	switch layer {
	case "domain":
		allowed = stdlib[importPath] || under(importPath, domain)
	case "usecase":
		allowed = stdlib[importPath] || under(importPath, domain) || under(importPath, usecase)
	case "inbound":
		allowed = !under(importPath, module+"/internal") || under(importPath, domain) || under(importPath, usecase) || under(importPath, inbound)
	case "outbound":
		allowed = !under(importPath, module+"/internal") || under(importPath, domain) || under(importPath, usecase) || under(importPath, outbound)
		if under(rel, "internal/adapters/outbound/fileconfig") && under(importPath, module+"/internal/config") {
			allowed = true
		}
		if under(rel, "internal/adapters/outbound/profilevalidation") && under(importPath, module+"/internal/profile") {
			allowed = true
		}
	case "bootstrap":
		return ""
	}
	if allowed {
		return ""
	}
	return layer + " may not depend on this package"
}

func under(importPath, prefix string) bool {
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}
