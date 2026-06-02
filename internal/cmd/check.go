package cmd

import (
	"fmt"
	"os"
	"sync"

	"github.com/lepeshko/keys/internal/loader"
	"github.com/lepeshko/keys/internal/version"
)

type checkResult struct {
	tool      loader.Tool
	installed string
	latest    string
	err       string
}

func RunCheck(toolName string, all bool, outdatedOnly bool) error {
	tools, err := loader.Load()
	if err != nil {
		return fmt.Errorf("cannot load tools: %v", err)
	}

	var targets []loader.Tool
	if all || outdatedOnly {
		targets = tools
	} else {
		for _, t := range tools {
			if t.Name == toolName {
				targets = []loader.Tool{t}
				break
			}
		}
		if len(targets) == 0 {
			return fmt.Errorf("tool %q not found\nrun 'keys list' to see available tools", toolName)
		}
	}

	results := make([]checkResult, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t loader.Tool) {
			defer wg.Done()
			r := checkResult{tool: t}
			r.installed = version.InstalledVersion(t)
			if t.GitHub != "" {
				var fetchErr error
				r.latest, fetchErr = version.FetchAndCache(t.GitHub)
				if fetchErr != nil {
					r.err = fetchErr.Error()
				}
			}
			results[i] = r
		}(i, t)
	}
	wg.Wait()

	nameW := 0
	for _, r := range results {
		if len(r.tool.Name) > nameW {
			nameW = len(r.tool.Name)
		}
	}

	printed := 0
	for _, r := range results {
		outdated := version.IsNewer(r.installed, r.latest)
		if outdatedOnly && !outdated {
			continue
		}

		status := formatStatus(r.installed, r.latest, r.err)
		fmt.Fprintf(os.Stdout, "%-*s  %s\n", nameW, r.tool.Name, status)
		printed++
	}

	if outdatedOnly && printed == 0 {
		fmt.Println("All tools are up to date.")
	}
	return nil
}

func formatStatus(installed, latest, errMsg string) string {
	switch {
	case installed == "" && latest == "" && errMsg == "":
		return "not installed"
	case installed == "" && latest == "" && errMsg != "":
		return "not installed  (latest: error — " + errMsg + ")"
	case installed == "" && latest != "":
		return "not installed  (latest: " + latest + ")"
	case installed != "" && errMsg != "":
		return "installed: " + installed + "  latest: error — " + errMsg
	case installed != "" && latest == "":
		return "installed: " + installed
	case version.IsNewer(installed, latest):
		return "installed: " + installed + "  latest: " + latest + "  ↑ update available"
	default:
		return "installed: " + installed + "  ✓ up to date"
	}
}
