package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// 从go.mod文件获取Go版本
func getGoVersionFromMod() (string, error) {
	// 检查go.mod文件是否存在
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		return "", fmt.Errorf("go.mod file not found in current directory")
	}

	file, err := os.Open("go.mod")
	if err != nil {
		return "", fmt.Errorf("failed to open go.mod: %v", err)
	}
	defer file.Close()

	// 逐行扫描go.mod文件
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// 查找go版本行，格式如 "go 1.20"
		if strings.HasPrefix(line, "go ") {
			version := strings.TrimSpace(strings.TrimPrefix(line, "go"))
			return version, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading go.mod: %v", err)
	}

	return "", fmt.Errorf("go version not found in go.mod")
}

// 解析Go版本为整数元组
func parseGoVersion(version string) ([]int, error) {
	parts := strings.Split(version, ".")
	result := make([]int, 0, len(parts))

	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid version part: %s", part)
		}
		result = append(result, num)
	}

	// 如果版本只有两个部分，添加0作为补充
	if len(result) < 3 {
		result = append(result, 0)
	}

	return result, nil
}

// 比较两个版本大小
func compareVersions(v1, v2 []int) int {
	minLen := len(v1)
	if len(v2) < minLen {
		minLen = len(v2)
	}

	for i := 0; i < minLen; i++ {
		if v1[i] < v2[i] {
			return -1
		}
		if v1[i] > v2[i] {
			return 1
		}
	}

	if len(v1) < len(v2) {
		return -1
	}
	if len(v1) > len(v2) {
		return 1
	}

	return 0
}

// 获取模块版本信息
type ModuleVersion struct {
	Version   string `json:"Version"`
	GoVersion string `json:"GoVersion"`
}

// 获取模块所有可用版本
func getModuleVersions(module string) ([]ModuleVersion, error) {
	// 查询模块所有版本
	cmd := exec.Command("go", "list", "-m", "-versions", "-json", module)
	output, err := cmd.Output()
	if err != nil {
		// 如果失败，尝试获取最新版本
		cmd = exec.Command("go", "list", "-m", "-json", module+"@latest")
		output, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get module info: %v", err)
		}

		var moduleInfo ModuleVersion
		if err := json.Unmarshal(output, &moduleInfo); err != nil {
			return nil, fmt.Errorf("failed to parse module info: %v", err)
		}

		// 获取该版本的Go版本要求
		cmd = exec.Command("go", "list", "-m", "-json", fmt.Sprintf("%s@%s", module, moduleInfo.Version))
		output, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("failed to get module detailed info: %v", err)
		}

		if err := json.Unmarshal(output, &moduleInfo); err != nil {
			return nil, fmt.Errorf("failed to parse module detailed info: %v", err)
		}

		return []ModuleVersion{moduleInfo}, nil
	}

	var moduleInfo struct {
		Versions []string `json:"Versions"`
	}
	if err := json.Unmarshal(output, &moduleInfo); err != nil {
		return nil, fmt.Errorf("failed to parse module versions: %v", err)
	}

	// 获取每个版本的Go版本要求
	result := make([]ModuleVersion, 0, len(moduleInfo.Versions))
	for _, version := range moduleInfo.Versions {
		cmd = exec.Command("go", "list", "-m", "-json", fmt.Sprintf("%s@%s", module, version))
		output, err = cmd.Output()
		if err != nil {
			continue
		}

		var verInfo ModuleVersion
		if err := json.Unmarshal(output, &verInfo); err != nil {
			continue
		}

		result = append(result, verInfo)
	}

	return result, nil
}

// 获取模块最新兼容版本
func getLatestCompatibleVersion(module string, goVersion string) (string, error) {
	// 解析当前Go版本
	currentGoVer, err := parseGoVersion(goVersion)
	if err != nil {
		return "", fmt.Errorf("invalid Go version %s: %v", goVersion, err)
	}

	// 获取模块的所有版本
	versions, err := getModuleVersions(module)
	if err != nil {
		return "", err
	}

	// 过滤和排序兼容版本
	compatibleVersions := make([]ModuleVersion, 0)
	for _, mv := range versions {
		if mv.GoVersion == "" {
			// 如果没有明确的Go版本要求，假定兼容
			compatibleVersions = append(compatibleVersions, mv)
			continue
		}

		// 提取Go版本要求的数字部分，格式通常是"go1.XX"
		re := regexp.MustCompile(`go(\d+\.\d+(\.\d+)?)`)
		matches := re.FindStringSubmatch(mv.GoVersion)
		if len(matches) < 2 {
			// 无法解析Go版本要求，假定兼容
			compatibleVersions = append(compatibleVersions, mv)
			continue
		}

		requiredGoVer, err := parseGoVersion(matches[1])
		if err != nil {
			continue
		}

		// 检查当前Go版本是否满足要求
		if compareVersions(currentGoVer, requiredGoVer) >= 0 {
			compatibleVersions = append(compatibleVersions, mv)
		}
	}

	if len(compatibleVersions) == 0 {
		return "", fmt.Errorf("no compatible version found for %s with Go %s", module, goVersion)
	}

	// 按模块版本排序，获取最新兼容版本
	sort.Slice(compatibleVersions, func(i, j int) bool {
		// 提取版本号（去掉前缀v）
		v1 := strings.TrimPrefix(compatibleVersions[i].Version, "v")
		v2 := strings.TrimPrefix(compatibleVersions[j].Version, "v")

		// 解析并比较版本
		ver1, err1 := parseGoVersion(v1)
		ver2, err2 := parseGoVersion(v2)

		if err1 != nil || err2 != nil {
			// 如果无法解析，使用字符串比较
			return compatibleVersions[i].Version < compatibleVersions[j].Version
		}

		return compareVersions(ver1, ver2) < 0
	})

	// 返回最新兼容版本
	latestCompatible := compatibleVersions[len(compatibleVersions)-1]
	fmt.Printf("Selected version %s (requires Go %s) compatible with your Go %s\n",
		latestCompatible.Version,
		latestCompatible.GoVersion,
		goVersion)

	return latestCompatible.Version, nil
}

// 执行go get命令
func executeGoGet(module string, version string) error {
	moduleWithVersion := module
	if version != "" {
		moduleWithVersion = fmt.Sprintf("%s@%s", module, version)
	}

	fmt.Printf("Getting %s...\n", moduleWithVersion)
	cmd := exec.Command("go", "get", moduleWithVersion)

	// 设置标准输出和标准错误到终端
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "go-get [module]",
		Short: "Automatically get the latest compatible version of a Go module",
		Long:  `A tool that determines the latest version of a Go module compatible with your current Go version and runs 'go get' for you.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			module := args[0]

			// 从go.mod获取Go版本
			goVersion, err := getGoVersionFromMod()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("Go version from go.mod: %s\n", goVersion)

			// 获取模块兼容版本
			version, err := getLatestCompatibleVersion(module, goVersion)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("Latest compatible version of %s: %s\n", module, version)

			// 执行go get
			if err := executeGoGet(module, version); err != nil {
				fmt.Printf("Failed to get module: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Successfully installed %s@%s\n", module, version)
		},
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
