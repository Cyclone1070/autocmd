package internal

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mstrYoda/go-arctest/pkg/arctest"
	"github.com/stretchr/testify/assert"
)

func TestArchitecture(t *testing.T) {
	// Base path should be the module root (one level up from internal)
	arch, err := arctest.New("..")
	if err != nil {
		t.Fatalf("failed to create architecture: %v", err)
	}
	if err := arch.ParsePackages(); err != nil {
		t.Fatalf("failed to parse packages: %v", err)
	}
	
	cwd, _ := os.Getwd()
	fmt.Printf("Debug: Working Dir: %s\n", cwd)

	module := "github.com/Cyclone1070/iav"
	var rules []*arctest.DependencyRule

	// Helper to create anchored regex patterns for packages
	pkg := func(p string) string {
		// Escape dots for regex
		p = strings.ReplaceAll(p, ".", "\\.")
		return fmt.Sprintf("^%s(/.*)?$", p)
	}

	// 1. Workflow Isolation
	// Must NOT import internal/ui, internal/cmd, or any internal services (excluding Exceptions).
	workflowForbidden := []string{
		module + "/internal/ui",
		module + "/cmd",
		module + "/internal/agent",
		module + "/internal/auth",
		module + "/internal/config",
		module + "/internal/fs",
		module + "/internal/provider",
		module + "/internal/session",
		module + "/internal/state",
		module + "/internal/tool/directory",
		module + "/internal/tool/file",
		module + "/internal/tool/search",
		module + "/internal/tool/shell",
		module + "/internal/tool/todo",
	}
	for _, target := range workflowForbidden {
		rule, err := arch.DoesNotDependOn(pkg("internal/workflow"), pkg(target))
		if err != nil {
			t.Fatalf("failed to create workflow rule: %v", err)
		}
		rules = append(rules, rule)
	}

	// 2. UI Isolation
	// Must NOT import internal/workflow, internal/cmd, or any internal services (excluding Exceptions).
	uiForbidden := []string{
		module + "/internal/workflow",
		module + "/cmd",
		module + "/internal/agent",
		module + "/internal/auth",
		module + "/internal/config",
		module + "/internal/fs",
		module + "/internal/provider",
		module + "/internal/session",
		module + "/internal/state",
	}
	for _, target := range uiForbidden {
		rule, err := arch.DoesNotDependOn(pkg("internal/ui"), pkg(target))
		if err != nil {
			t.Fatalf("failed to create ui rule: %v", err)
		}
		rules = append(rules, rule)
	}

	// 3. Service Isolation
	services := []string{
		"agent", "auth", "config", "fs", "provider", "session", "state",
		"tool/directory", "tool/file", "tool/search", "tool/shell", "tool/todo",
	}
	for _, service := range services {
		serviceRelPath := "internal/" + service
		
		// Services cannot import workflow, ui, or cmd
		serviceForbidden := []string{
			module + "/internal/workflow",
			module + "/internal/ui",
			module + "/cmd",
		}
		for _, target := range serviceForbidden {
			rule, err := arch.DoesNotDependOn(pkg(serviceRelPath), pkg(target))
			if err != nil {
				t.Fatalf("failed to create service rule: %v", err)
			}
			rules = append(rules, rule)
		}

		// Services cannot import each other
		for _, other := range services {
			if service == other {
				continue
			}
			rule, err := arch.DoesNotDependOn(pkg(serviceRelPath), pkg(module+"/internal/"+other))
			if err != nil {
				t.Fatalf("failed to create service cross-dependency rule: %v", err)
			}
			rules = append(rules, rule)
		}
	}

	// Validate
	success, violations := arch.ValidateDependenciesWithRules(rules)
	if !success {
		// Group violations by source package for clearer reporting
		fmt.Printf("\n--- ARCHITECTURE VIOLATIONS DETECTED ---\n")
		
		// We use a map to deduplicate (arctest might report the same violation multiple times if it appears in multiple files)
		seen := make(map[string]bool)
		for _, v := range violations {
			if seen[v] {
				continue
			}
			seen[v] = true
			
			// Clean up output: remove internal/ prefix from source to make it more readable
			cleaned := strings.Replace(v, "Package \"internal/", "Package \"", 1)
			fmt.Printf("Violation: %s\n", cleaned)
		}
		fmt.Printf("----------------------------------------\n\n")
	}
	assert.True(t, success, "Architecture violations detected (see output above)")
}
