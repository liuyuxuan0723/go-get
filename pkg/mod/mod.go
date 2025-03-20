package mod

const defaultPath = ""

type Manager struct {
	modMap map[string]string
}

func NewManager() *Manager {
	return nil
}

func (m *Manager) GoGet(module string) error {
	return nil
}

func (m *Manager) getLocalGoVersion() (string, error) {
	return "", nil
}

func (m *Manager) findVersion(module string) (string, error) {
	return "", nil
}

func (m *Manager) executeGoGet(module, version string) error {
	return nil
}

func (m *Manager) loadCache() {

}
