package mod

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultPath = "go.mod"

type Manager struct {
	modMap    map[string]map[string]string
	cachePath string
}

type modInfo struct {
	Path      string `json:"Path"`
	Version   string `json:"Version"`
	Time      string `json:"Time"`
	GoMod     string `json:"GoMod"`
	GoVersion string `json:"GoVersion"`
}

func NewManager() *Manager {
	homeDir, err := os.UserHomeDir()
	cachePath := filepath.Join(homeDir, ".gomod_cache.json")
	if err != nil {
		log.Printf("获取用户目录失败: %v, 使用当前目录", err)
		cachePath = ".gomod_cache.json"
	}
	log.Printf("缓存路径: %s", cachePath)

	return &Manager{
		modMap:    make(map[string]map[string]string),
		cachePath: cachePath,
	}
}

func (m *Manager) GoGet(module string) error {
	log.Printf("开始安装模块: %s", module)
	if m.modMap == nil {
		log.Printf("初始化模块映射")
		m.loadCache()
	}

	localVersion, err := m.getLocalGoVersion()
	if err != nil {
		log.Printf("获取本地Go版本失败: %v", err)
		return fmt.Errorf("获取本地Go版本失败: %w", err)
	}
	log.Printf("本地Go版本: %s", localVersion)

	moduleVersion, err := m.findVersion(localVersion, module)
	if err != nil {
		log.Printf("查找模块版本失败: %v", err)
		return fmt.Errorf("查找模块版本失败: %w", err)
	}
	log.Printf("找到兼容版本: %s@%s", module, moduleVersion)

	if err := m.executeGoGet(module, moduleVersion); err != nil {
		log.Printf("执行go get失败: %v", err)
		return fmt.Errorf("执行go get失败: %w", err)
	}
	log.Printf("成功安装模块: %s@%s", module, moduleVersion)

	if m.modMap[localVersion] == nil {
		m.modMap[localVersion] = make(map[string]string)
	}
	m.modMap[localVersion][module] = moduleVersion
	log.Printf("更新缓存: %s -> %s@%s", localVersion, module, moduleVersion)

	return m.saveCache()
}

func (m *Manager) getLocalGoVersion() (string, error) {
	data, err := os.ReadFile(defaultPath)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "go ") {
				version := "go" + strings.TrimSpace(strings.TrimPrefix(line, "go "))
				log.Printf("从go.mod获取Go版本: %s", version)
				return version, nil
			}
		}
		log.Printf("go.mod文件中未找到Go版本")
	} else {
		log.Printf("读取go.mod失败: %v", err)
	}

	cmd := exec.Command("go", "version")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("执行go version命令失败: %v", err)
		return "", fmt.Errorf("执行go version命令失败: %w", err)
	}

	versionStr := string(output)
	parts := strings.Split(versionStr, " ")
	if len(parts) < 3 {
		log.Printf("无法解析Go版本: %s", versionStr)
		return "", fmt.Errorf("无法解析Go版本: %s", versionStr)
	}

	log.Printf("从go version命令获取Go版本: %s", parts[2])
	return parts[2], nil
}

func (m *Manager) findVersion(goVersion, module string) (string, error) {
	log.Printf("查找模块版本: %s (Go %s)", module, goVersion)
	if m.modMap[goVersion] != nil && m.modMap[goVersion][module] != "" {
		log.Printf("从缓存获取版本: %s", m.modMap[goVersion][module])
		return m.modMap[goVersion][module], nil
	}

	cmd := exec.Command("go", "list", "-m", "-versions", module)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("查询模块版本列表失败: %v", err)
		return "", fmt.Errorf("查询模块版本列表失败: %w", err)
	}

	versionsStr := strings.TrimSpace(string(output))
	parts := strings.Split(versionsStr, " ")
	if len(parts) <= 1 {
		log.Printf("模块没有版本列表，尝试获取最新版本")
		cmd = exec.Command("go", "list", "-m", "-json", module+"@latest")
		output, err = cmd.Output()
		if err != nil {
			log.Printf("查询模块最新版本失败: %v", err)
			return "", fmt.Errorf("查询模块最新版本失败: %w", err)
		}

		var info modInfo
		if err = json.Unmarshal(output, &info); err != nil {
			log.Printf("解析模块信息失败: %v", err)
			return "", fmt.Errorf("解析模块信息失败: %w", err)
		}

		if info.Version == "" {
			log.Printf("无法获取模块版本")
			return "", fmt.Errorf("无法获取模块 %s 的版本", module)
		}

		if m.check(goVersion, module, info.Version) {
			log.Printf("最新版本兼容: %s", info.Version)
			return info.Version, nil
		} else {
			log.Printf("最新版本不兼容: %s", info.Version)
			return "", fmt.Errorf("模块 %s 的最新版本 %s 与当前 Go 版本 %s 不兼容",
				module, info.Version, goVersion)
		}
	}

	// https://goproxy.io/k8s.io/client-go/@v

	versions := parts[1:]
	log.Printf("找到 %d 个版本", len(versions))

	for i := len(versions) - 1; i >= 0; i-- {
		version := versions[i]
		log.Printf("检查版本兼容性: %s", version)
		if m.check(goVersion, module, version) {
			log.Printf("找到兼容版本: %s", version)
			return version, nil
		}
		log.Printf("版本不兼容: %s", version)
	}

	log.Printf("没有找到兼容版本")
	return "", fmt.Errorf("没有找到与 Go %s 兼容的 %s 版本", goVersion, module)
}

func (m *Manager) check(goVersion, module, moduleVersion string) bool {
	log.Printf("检查兼容性: %s@%s 与 Go %s", module, moduleVersion, goVersion)
	cmd := exec.Command("go", "list", "-m", "-json", module+"@"+moduleVersion)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("获取模块信息失败: %v", err)
		return false
	}

	var info modInfo
	if err = json.Unmarshal(output, &info); err != nil {
		log.Printf("解析模块信息失败: %v", err)
		return false
	}

	if info.GoVersion == "" {
		log.Printf("模块未指定Go版本要求，假设兼容")
		return true
	}

	log.Printf("模块要求Go版本: %s", info.GoVersion)
	result := compareGoVersions(goVersion, info.GoVersion)
	log.Printf("兼容性检查结果: %v", result)
	return result
}

func compareGoVersions(currentVersion, requiredVersion string) bool {
	current := strings.TrimPrefix(currentVersion, "go")
	required := strings.TrimPrefix(requiredVersion, "go")

	currentFloat, err1 := strconv.ParseFloat(current, 64)
	requiredFloat, err2 := strconv.ParseFloat(required, 64)

	if err1 != nil || err2 != nil {
		log.Printf("版本解析失败: %v, %v", err1, err2)
		return true
	}

	log.Printf("版本比较: %f >= %f: %v", currentFloat, requiredFloat, currentFloat >= requiredFloat)
	return currentFloat >= requiredFloat
}

func (m *Manager) executeGoGet(module, version string) error {
	log.Printf("执行: go get %s@%s", module, version)
	cmd := exec.Command("go", "get", module+"@"+version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		log.Printf("go get命令执行失败: %v", err)
	} else {
		log.Printf("go get命令执行成功")
	}
	return err
}

func (m *Manager) loadCache() {
	log.Printf("加载缓存: %s", m.cachePath)
	m.modMap = make(map[string]map[string]string)

	data, err := os.ReadFile(m.cachePath)
	if err != nil {
		log.Printf("读取缓存文件失败: %v", err)
		return
	}

	if err = json.Unmarshal(data, &m.modMap); err != nil {
		log.Printf("解析缓存失败: %v", err)
		m.modMap = make(map[string]map[string]string)
	} else {
		log.Printf("成功加载缓存，包含 %d 个Go版本", len(m.modMap))
	}
}

func (m *Manager) saveCache() error {
	log.Printf("保存缓存: %s", m.cachePath)
	data, err := json.MarshalIndent(m.modMap, "", "  ")
	if err != nil {
		log.Printf("序列化缓存失败: %v", err)
		return fmt.Errorf("序列化缓存失败: %w", err)
	}

	if err := os.WriteFile(m.cachePath, data, 0644); err != nil {
		log.Printf("写入缓存文件失败: %v", err)
		return fmt.Errorf("写入缓存文件失败: %w", err)
	}

	log.Printf("缓存保存成功")
	return nil
}
