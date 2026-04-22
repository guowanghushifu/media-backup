package watcher

type Registrar interface {
	Add(path string) error
}

type MemoryRegistrar struct {
	paths []string
}

func NewMemoryRegistrar() *MemoryRegistrar {
	return &MemoryRegistrar{}
}

func (m *MemoryRegistrar) Add(path string) error {
	m.paths = append(m.paths, path)
	return nil
}

func (m *MemoryRegistrar) Paths() []string {
	return append([]string(nil), m.paths...)
}

type WatchService struct {
	reg Registrar
}

func NewWatchService(reg Registrar) *WatchService {
	return &WatchService{reg: reg}
}

func (s *WatchService) AddRecursive(root string, existing []string) error {
	for _, path := range existing {
		if err := s.reg.Add(path); err != nil {
			return err
		}
	}
	return nil
}
