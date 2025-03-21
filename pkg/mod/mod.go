package mod

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
)

var (
	GoVersionPattern    = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(\.\d+)?)`)
	GoCmdVersionPattern = regexp.MustCompile(`go(\d+\.\d+(\.\d+)?)`)
)

const MaxConcurrent = 10

type VersionResult struct {
	Version    string
	GoVersion  string
	Compatible bool
}

type Manager struct {
	modMap    map[string]map[string]string
	cachePath string
	verbose   bool
}

func NewManager(verbose bool) *Manager {
	homeDir, err := os.UserHomeDir()
	cachePath := filepath.Join(homeDir, ".gomod_cache.json")
	if err != nil {
		cachePath = ".gomod_cache.json"
	}

	return &Manager{
		modMap:    make(map[string]map[string]string),
		cachePath: cachePath,
		verbose:   verbose,
	}
}

func (m *Manager) logInfo(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func (m *Manager) logDebug(format string, v ...interface{}) {
	if m.verbose {
		log.Printf(format, v...)
	}
}

func (m *Manager) GoVersion() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err == nil {
		matches := GoVersionPattern.FindStringSubmatch(string(data))
		if len(matches) > 1 {
			version := "go" + matches[1]
			m.logDebug("Go version from go.mod: %s", version)
			return version, nil
		}
	}

	output, err := exec.Command("go", "version").Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute go version command: %w", err)
	}

	matches := GoCmdVersionPattern.FindStringSubmatch(string(output))
	if len(matches) > 1 {
		version := "go" + matches[1]
		m.logDebug("Go version from command: %s", version)
		return version, nil
	}

	return "", fmt.Errorf("unable to parse Go version: %s", string(output))
}

func (m *Manager) GoGet(module string) error {
	localGoVersion, err := m.GoVersion()
	if err != nil {
		return fmt.Errorf("failed to get local Go version: %w", err)
	}

	m.loadCache()

	versions, err := listVersion(module, m.verbose)
	if err != nil || len(versions) == 0 {
		return fmt.Errorf("failed to get available versions for %s: %w", module, err)
	}
	m.logInfo("Module %s has %d available versions", module, len(versions))

	compatibleVersion, err := m.findCompatibleVersion(module, versions, localGoVersion)
	if err != nil {
		return err
	}

	m.logInfo("Executing: go get %s@%s", module, compatibleVersion)
	cmd := exec.Command("go", "get", fmt.Sprintf("%s@%s", module, compatibleVersion))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute go get command: %w", err)
	}

	m.logInfo("Successfully installed %s@%s", module, compatibleVersion)
	return nil
}

func (m *Manager) findCompatibleVersion(module string, versions []string, localGoVersion string) (string, error) {
	if compatibleVersion := m.findCompatibleVersionFromCache(module, versions, localGoVersion); compatibleVersion != "" {
		return compatibleVersion, nil
	}

	m.logDebug("No compatible version found in cache, checking remotely")

	if _, exists := m.modMap[module]; !exists {
		m.modMap[module] = make(map[string]string)
	}

	resultMap := make(map[string]VersionResult)
	resultChan := make(chan VersionResult, len(versions))

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxConcurrent)

	for i := len(versions) - 1; i >= 0; i-- {
		wg.Add(1)
		go func(version string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			goVer, err := getModuleGoVersion(module, version, m.verbose)
			if err != nil {
				m.logDebug("Failed to get Go requirement for version %s: %v", version, err)
				resultChan <- VersionResult{Version: version, GoVersion: "", Compatible: false}
				return
			}

			m.modMap[module][version] = goVer
			compatible := goVer == "" || compareGoVersions(localGoVersion, "go"+goVer)

			if compatible {
				m.logDebug("Found compatible version: %s (Go requirement: %s)", version, goVer)
			}

			resultChan <- VersionResult{Version: version, GoVersion: goVer, Compatible: compatible}
		}(versions[i])
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		resultMap[result.Version] = result
	}

	m.saveCache()

	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if result, ok := resultMap[v]; ok && result.Compatible {
			m.logDebug("Selected latest compatible version: %s (Go requirement: %s)", v, result.GoVersion)
			return v, nil
		}
	}

	return "", fmt.Errorf("no compatible version found for %s with Go %s", module, localGoVersion)
}

func (m *Manager) findCompatibleVersionFromCache(module string, versions []string, localGoVersion string) string {
	versionMap, exists := m.modMap[module]
	if !exists {
		return ""
	}

	m.logDebug("Getting module info from cache")

	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if goVer, ok := versionMap[v]; ok {
			if goVer == "" || compareGoVersions(localGoVersion, "go"+goVer) {
				m.logDebug("Found compatible version in cache: %s (Go requirement: %s)", v, goVer)
				return v
			}
		}
	}

	return ""
}

func (m *Manager) GoModTidy() error {
	return nil
}

func (m *Manager) loadCache() {
	m.logDebug("Loading cache: %s", m.cachePath)
	m.modMap = make(map[string]map[string]string)

	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		m.logDebug("Failed to read cache file: %v, creating new cache", err)
		emptyCache, _ := json.MarshalIndent(m.modMap, "", "  ")
		os.WriteFile(m.cachePath, emptyCache, 0644)
		return
	}

	if err = json.Unmarshal(data, &m.modMap); err != nil {
		m.logDebug("Failed to parse cache: %v, resetting cache", err)
		m.modMap = make(map[string]map[string]string)
		emptyCache, _ := json.MarshalIndent(m.modMap, "", "  ")
		os.WriteFile(m.cachePath, emptyCache, 0644)
	} else {
		m.logDebug("Successfully loaded cache with %d modules", len(m.modMap))
	}
}

func (m *Manager) saveCache() error {
	m.logDebug("Saving cache: %s", m.cachePath)
	data, err := json.MarshalIndent(m.modMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize cache: %w", err)
	}

	if err := os.WriteFile(m.cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	m.logDebug("Cache saved successfully")
	return nil
}
