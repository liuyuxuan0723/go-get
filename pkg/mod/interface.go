package mod

type Interface interface {
	GoVersion() (string, error)
	GoGet(module string) error
	GoModTidy() error
}
