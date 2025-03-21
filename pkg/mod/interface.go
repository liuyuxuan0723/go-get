package mod

type Interface interface {
	GoVersion() (string, error)
	GoGet(module string) error
	// 根据本地或 go.mod 中的 go 版本，获取兼容的版本
	GoModTidy() error
}
