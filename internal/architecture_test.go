package internal

import (
	"fmt"
	"os"
	"regexp"
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
	toolServices := []string{
		"internal/tool/bash",
		"internal/tool/edit",
		"internal/tool/glob",
		"internal/tool/grep",
		"internal/tool/question",
		"internal/tool/read",
		"internal/tool/write",
	}

	// Source packages are stored as relative paths like "internal/agent".
	sourcePkg := func(p string) string {
		return fmt.Sprintf("^%s(/.*)?$", regexp.QuoteMeta(p))
	}
	// Imported targets are full import paths like "github.com/Cyclone1070/iav/internal/permission".
	targetPkg := func(p string) string {
		if !strings.HasPrefix(p, module) {
			p = module + "/" + strings.TrimPrefix(p, "/")
		}
		return fmt.Sprintf("^%s(/.*)?$", regexp.QuoteMeta(p))
	}

	// 1. Workflow Isolation
	// Must NOT import internal/ui, internal/cmd, or any internal services (excluding Exceptions).
	workflowForbidden := make([]string, 0, 12+len(toolServices))
	workflowForbidden = append(workflowForbidden,
		"internal/ui",
		"cmd",
		"internal/actionrouter",
		"internal/agent",
		"internal/auth",
		"internal/config",
		"internal/eventbus",
		"internal/fs",
		"internal/permission",
		"internal/provider",
		"internal/session",
		"internal/state",
	)
	workflowForbidden = append(workflowForbidden, toolServices...)
	for _, target := range workflowForbidden {
		rule, err := arch.DoesNotDependOn(sourcePkg("internal/workflow"), targetPkg(target))
		if err != nil {
			t.Fatalf("failed to create workflow rule: %v", err)
		}
		rules = append(rules, rule)
	}

	// 2. UI Isolation
	// Must NOT import internal/workflow, internal/cmd, or any internal services (excluding Exceptions).
	uiForbidden := make([]string, 0, 12+len(toolServices))
	uiForbidden = append(uiForbidden,
		"internal/workflow",
		"cmd",
		"internal/actionrouter",
		"internal/agent",
		"internal/auth",
		"internal/config",
		"internal/eventbus",
		"internal/fs",
		"internal/permission",
		"internal/provider",
		"internal/session",
		"internal/state",
	)
	uiForbidden = append(uiForbidden, toolServices...)
	for _, target := range uiForbidden {
		rule, err := arch.DoesNotDependOn(sourcePkg("internal/ui"), targetPkg(target))
		if err != nil {
			t.Fatalf("failed to create ui rule: %v", err)
		}
		rules = append(rules, rule)
	}

	// 3. Service Isolation
	services := []string{
		"actionrouter",
		"agent",
		"auth",
		"config",
		"eventbus",
		"fs",
		"permission",
		"provider",
		"session",
		"state",
		"tool/bash",
		"tool/edit",
		"tool/glob",
		"tool/grep",
		"tool/question",
		"tool/read",
		"tool/write",
	}
	for _, service := range services {
		serviceRelPath := "internal/" + service

		// Services cannot import workflow, ui, or cmd
		serviceForbidden := []string{
			"internal/workflow",
			"internal/ui",
			"cmd",
		}
		for _, target := range serviceForbidden {
			rule, err := arch.DoesNotDependOn(sourcePkg(serviceRelPath), targetPkg(target))
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
			rule, err := arch.DoesNotDependOn(sourcePkg(serviceRelPath), targetPkg("internal/"+other))
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
