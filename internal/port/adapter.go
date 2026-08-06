package port

// WriteOptions is threaded from the orchestrator/ledger into write().
type WriteOptions struct {
	Force          bool
	ExistingDestID string
}

// ImportAdapter: harness → hub IR.
type ImportAdapter interface {
	Harness() HarnessID
	Detect() (bool, error)
	Discover() ([]SessionRef, error)
	Parse(ref SessionRef) (PortableSession, error)
}

// ExportAdapter: hub IR → harness.
type ExportAdapter interface {
	Harness() HarnessID
	Detect() (bool, error)
	Write(session PortableSession, opts WriteOptions) (ExportResult, error)
}

// Registry holds import/export adapters.
type Registry struct {
	imports map[HarnessID]ImportAdapter
	exports map[HarnessID]ExportAdapter
}

func NewRegistry() *Registry {
	return &Registry{
		imports: map[HarnessID]ImportAdapter{},
		exports: map[HarnessID]ExportAdapter{},
	}
}

func (r *Registry) RegisterImport(a ImportAdapter) { r.imports[a.Harness()] = a }
func (r *Registry) RegisterExport(a ExportAdapter) { r.exports[a.Harness()] = a }

func (r *Registry) Import(id HarnessID) (ImportAdapter, bool) {
	a, ok := r.imports[id]
	return a, ok
}

func (r *Registry) Export(id HarnessID) (ExportAdapter, bool) {
	a, ok := r.exports[id]
	return a, ok
}

func (r *Registry) ListImport() []HarnessID {
	var out []HarnessID
	for k := range r.imports {
		out = append(out, k)
	}
	return out
}

func (r *Registry) ListExport() []HarnessID {
	var out []HarnessID
	for k := range r.exports {
		out = append(out, k)
	}
	return out
}
