package mod

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
)

var (
	ModGoVersionPattern  = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(\.\d+)?)`)
	GoproxyCN            = "https://goproxy.cn/"
	StableVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
)

func listVersion(module string, verbose bool) ([]string, error) {
	url := fmt.Sprintf(GoproxyCN+"%s/@v/list", module)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	allVersions := strings.Split(strings.TrimSpace(string(body)), "\n")
	var stableVersions []string

	for _, v := range allVersions {
		if StableVersionPattern.MatchString(v) && !strings.Contains(v, "+incompatible") {
			stableVersions = append(stableVersions, v)
		}
	}

	if verbose {
		log.Printf("Module %s has %d total versions, %d stable versions", module, len(allVersions), len(stableVersions))
	}

	if len(stableVersions) == 0 {
		if verbose {
			log.Printf("Trying to use non-prerelease versions")
		}

		for _, v := range allVersions {
			if !strings.Contains(v, "-alpha") &&
				!strings.Contains(v, "-beta") &&
				!strings.Contains(v, "-rc") &&
				!strings.Contains(v, "+incompatible") {
				stableVersions = append(stableVersions, v)
			}
		}

		if len(stableVersions) == 0 && verbose {
			log.Printf("Using all available versions")
			return allVersions, nil
		}
	}

	return stableVersions, nil
}

func getModuleGoVersion(module, version string, verbose bool) (string, error) {
	url := fmt.Sprintf(GoproxyCN+"%s/@v/%s.mod", module, version)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	matches := ModGoVersionPattern.FindStringSubmatch(string(body))
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("no Go version found in go.mod for %s@%s", module, version)
}

func compareGoVersions(currentVersion, requiredVersion string) bool {
	current := strings.TrimPrefix(currentVersion, "go")
	required := strings.TrimPrefix(requiredVersion, "go")

	v1, err1 := version.NewVersion(current)
	v2, err2 := version.NewVersion(required)

	if err1 != nil || err2 != nil {
		return true
	}

	return v1.GreaterThanOrEqual(v2)
}
