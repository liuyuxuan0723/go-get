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

const MaxConcurrent = 20

type versionResult struct {
	version    string
	goVersion  string
	compatible bool
}

type Manager struct {
	modMap    map[string]map[string]string
	cachePath string
	verbose   bool
	mutex     sync.Mutex
	sem       chan struct{}
}

func NewManager(verbose bool) *Manager {
	homeDir, err := os.UserHomeDir()
	cachePath := filepath.Join(homeDir, ".mod_cache.json")
	if err != nil {
		cachePath = ".mod_cache.json"
	}

	return &Manager{
		modMap:    make(map[string]map[string]string),
		cachePath: cachePath,
		verbose:   verbose,
		sem:       make(chan struct{}, MaxConcurrent),
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

func (m *Manager) GoGet(module string, refresh bool) error {
	localGoVersion, err := m.GoVersion()
	if err != nil {
		return fmt.Errorf("failed to get local Go version: %w", err)
	}

	if err = m.loadCache(); err != nil {
		return err
	}

	versions, err := listVersion(module, m.verbose)
	if err != nil || len(versions) == 0 {
		return fmt.Errorf("failed to get available versions for %s: %w", module, err)
	}
	m.logInfo("Module %s has %d available versions", module, len(versions))

	var compatibleVersion string
	var findErr error

	if refresh {
		m.logInfo("Force refreshing cache for %s", module)
		compatibleVersion, findErr = m.findCompatibleVersionRemote(module, versions, localGoVersion)
	} else {
		compatibleVersion, findErr = m.findCompatibleVersion(module, versions, localGoVersion)
	}

	if findErr != nil {
		return findErr
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

	m.mutex.Lock()
	if _, exists := m.modMap[localGoVersion]; !exists {
		m.modMap[localGoVersion] = make(map[string]string)
	}
	m.mutex.Unlock()

	resultChan := make(chan versionResult, len(versions))
	var wg sync.WaitGroup

	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		wg.Add(1)
		go func(ver string) {
			defer wg.Done()

			m.sem <- struct{}{}
			defer func() { <-m.sem }()

			goVer, err := getModuleGoVersion(module, ver, m.verbose)
			if err != nil {
				m.logDebug("Failed to get Go requirement for version %s: %v", ver, err)
				resultChan <- versionResult{version: ver, compatible: false}
				return
			}

			compatible := goVer == "" || compareGoVersions(localGoVersion, "go"+goVer)

			if compatible {
				m.logDebug("Found compatible version: %s (Go requirement: %s)", ver, goVer)
			}

			resultChan <- versionResult{version: ver, goVersion: goVer, compatible: compatible}
		}(version)
	}

	wg.Wait()
	close(resultChan)

	compatibleVersions := make(map[string]bool)
	for result := range resultChan {
		if result.compatible {
			compatibleVersions[result.version] = true
		}
	}

	var selectedVersion string
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if compatibleVersions[v] {
			selectedVersion = v
			m.logDebug("Selected latest compatible version: %s", v)

			m.modMap[localGoVersion][module] = selectedVersion

			if err := m.saveCache(); err != nil {
				m.logDebug("Failed to save cache: %v", err)
			}

			return selectedVersion, nil
		}
	}

	return "", fmt.Errorf("no compatible version found for %s with Go %s", module, localGoVersion)
}

func (m *Manager) findCompatibleVersionFromCache(module string, versions []string, localGoVersion string) string {
	m.mutex.Lock()
	goVersionMap, exists := m.modMap[localGoVersion]
	m.mutex.Unlock()

	if !exists {
		return ""
	}

	m.logDebug("Getting module info from cache")

	m.mutex.Lock()
	cachedVersion, exists := goVersionMap[module]
	m.mutex.Unlock()

	if !exists {
		return ""
	}

	for _, v := range versions {
		if v == cachedVersion {
			m.logDebug("Found compatible version in cache: %s", cachedVersion)
			return cachedVersion
		}
	}

	return ""
}

func (m *Manager) findCompatibleVersionRemote(module string, versions []string, localGoVersion string) (string, error) {
	m.logDebug("Skipping cache, checking remotely for latest versions")

	m.mutex.Lock()
	if _, exists := m.modMap[localGoVersion]; !exists {
		m.modMap[localGoVersion] = make(map[string]string)
	}
	m.mutex.Unlock()

	resultChan := make(chan versionResult, len(versions))
	var wg sync.WaitGroup

	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		wg.Add(1)
		go func(ver string) {
			defer wg.Done()

			m.sem <- struct{}{}
			defer func() { <-m.sem }()

			goVer, err := getModuleGoVersion(module, ver, m.verbose)
			if err != nil {
				m.logDebug("Failed to get Go requirement for version %s: %v", ver, err)
				resultChan <- versionResult{version: ver, compatible: false}
				return
			}

			compatible := goVer == "" || compareGoVersions(localGoVersion, "go"+goVer)

			if compatible {
				m.logDebug("Found compatible version: %s (Go requirement: %s)", ver, goVer)
			}

			resultChan <- versionResult{version: ver, goVersion: goVer, compatible: compatible}
		}(version)
	}

	wg.Wait()
	close(resultChan)

	compatibleVersions := make(map[string]bool)
	for result := range resultChan {
		if result.compatible {
			compatibleVersions[result.version] = true
		}
	}

	var selectedVersion string
	for i := len(versions) - 1; i >= 0; i-- {
		v := versions[i]
		if compatibleVersions[v] {
			selectedVersion = v
			m.logDebug("Selected latest compatible version: %s", v)

			m.modMap[localGoVersion][module] = selectedVersion

			if err := m.saveCache(); err != nil {
				m.logDebug("Failed to save cache: %v", err)
			}

			return selectedVersion, nil
		}
	}

	return "", fmt.Errorf("no compatible version found for %s with Go %s", module, localGoVersion)
}

func (m *Manager) GoModTidy() error {
	return nil
}

func (m *Manager) loadCache() error {
	m.logDebug("Loading cache: %s", m.cachePath)
	m.modMap = make(map[string]map[string]string)

	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		m.logDebug("Failed to read cache file: %v, creating new cache", err)
		emptyCache, _ := json.MarshalIndent(m.modMap, "", "  ")

		return os.WriteFile(m.cachePath, emptyCache, 0644)
	}

	if err = json.Unmarshal(data, &m.modMap); err != nil {
		m.logDebug("Failed to parse cache: %v, resetting cache", err)
		m.modMap = make(map[string]map[string]string)
		emptyCache, _ := json.MarshalIndent(m.modMap, "", "  ")
		return os.WriteFile(m.cachePath, emptyCache, 0644)
	} else {
		m.logDebug("Successfully loaded cache with %d modules", len(m.modMap))
	}
	return nil
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
