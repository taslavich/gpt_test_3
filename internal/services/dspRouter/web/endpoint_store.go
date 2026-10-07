package dspRouterWeb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/filter"
	sppAdapterWeb "gitlab.com/twinbid-exchange/RTB-exchange/internal/services/sspAdapter/web"
)

// DSPEndpointMap maps an HTTP OpenRTB endpoint to the DSP domain used by the
// existing routing/filter/statistics pipeline.
type DSPEndpointMap map[string]string

var ErrInvalidDSPEndpointMap = errors.New("invalid DSP endpoints map")

type dspEndpointRuntime struct {
	mapa       DSPEndpointMap
	ordered    []DSPEndpointV25
	nativeMask filter.NativeFieldMask
}

// DSPHTTPClientRegistry preserves the old router behavior where one HTTP client
// is shared by all endpoints carrying the same DSP domain, even across formats.
// It also lets hot-added domains obtain a client before the runtime snapshot is
// published.
type DSPHTTPClientRegistry struct {
	mu      sync.Mutex
	clients map[string]*http.Client
}

func NewDSPHTTPClientRegistry() *DSPHTTPClientRegistry {
	return &DSPHTTPClientRegistry{clients: make(map[string]*http.Client)}
}

func (r *DSPHTTPClientRegistry) Client(domain string) *http.Client {
	if r == nil {
		return NewFastHTTPClient()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if client := r.clients[domain]; client != nil {
		return client
	}
	client := NewFastHTTPClient()
	r.clients[domain] = client
	return client
}

// DSPEndpointStore mirrors the SSP feed-store control-plane model: the JSON
// file is durable state, while request handling reads one immutable runtime
// snapshot through atomic.Value. PUT builds and validates the complete next
// snapshot before touching disk or runtime.
type DSPEndpointStore struct {
	updateMu  sync.Mutex
	filename  string
	typic     string
	processor *filter.OptimizedFilterProcessor
	registry  *DSPHTTPClientRegistry
	runtime   atomic.Value // *dspEndpointRuntime
}

func NewDSPEndpointStore(filename, typic string, processor *filter.OptimizedFilterProcessor, registries ...*DSPHTTPClientRegistry) (*DSPEndpointStore, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, errors.New("DSP endpoints map filename is empty")
	}
	if typic != sppAdapterWeb.ADULT && typic != sppAdapterWeb.MAINSTREAM {
		return nil, fmt.Errorf("invalid DSP endpoints typic %q", typic)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read DSP endpoints map %s: %w", filename, err)
	}
	mapa, err := decodeDSPEndpointMap(data)
	if err != nil {
		return nil, fmt.Errorf("parse DSP endpoints map %s: %w", filename, err)
	}

	registry := NewDSPHTTPClientRegistry()
	if len(registries) > 0 && registries[0] != nil {
		registry = registries[0]
	}
	store := &DSPEndpointStore{filename: filename, typic: typic, processor: processor, registry: registry}
	runtime, err := store.buildRuntime(mapa)
	if err != nil {
		return nil, fmt.Errorf("validate DSP endpoints map %s: %w", filename, err)
	}
	store.runtime.Store(runtime)
	return store, nil
}

func decodeDSPEndpointMap(data []byte) (DSPEndpointMap, error) {
	if strings.TrimSpace(string(data)) == "" {
		return DSPEndpointMap{}, nil
	}
	var mapa DSPEndpointMap
	if err := json.Unmarshal(data, &mapa); err != nil {
		return nil, err
	}
	if mapa == nil {
		return DSPEndpointMap{}, nil
	}
	return mapa, nil
}

func cloneDSPEndpointMap(input DSPEndpointMap) DSPEndpointMap {
	out := make(DSPEndpointMap, len(input))
	for endpoint, domain := range input {
		out[endpoint] = domain
	}
	return out
}

func validateDSPEndpoint(endpoint, domain, typic string) error {
	endpoint = strings.TrimSpace(endpoint)
	domain = strings.TrimSpace(domain)
	if endpoint == "" || domain == "" {
		return errors.New("endpoint and domain must be non-empty")
	}
	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed == nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid DSP endpoint URL %q", endpoint)
	}

	switch typic {
	case sppAdapterWeb.ADULT:
		if !strings.HasPrefix(domain, ADL_PREFIX) || len(domain) == len(ADL_PREFIX) {
			return fmt.Errorf("adult DSP domain %q must start with %q", domain, ADL_PREFIX)
		}
	case sppAdapterWeb.MAINSTREAM:
		if !strings.HasPrefix(domain, MC_PREFIX) || len(domain) == len(MC_PREFIX) {
			return fmt.Errorf("mainstream DSP domain %q must start with %q", domain, MC_PREFIX)
		}
	default:
		return fmt.Errorf("invalid typic %q", typic)
	}
	return nil
}

func (s *DSPEndpointStore) buildRuntime(mapa DSPEndpointMap) (*dspEndpointRuntime, error) {
	if mapa == nil {
		mapa = DSPEndpointMap{}
	}
	clean := make(DSPEndpointMap, len(mapa))
	for rawEndpoint, rawDomain := range mapa {
		endpoint := strings.TrimSpace(rawEndpoint)
		domain := strings.TrimSpace(rawDomain)
		if err := validateDSPEndpoint(endpoint, domain, s.typic); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidDSPEndpointMap, err)
		}
		if _, exists := clean[endpoint]; exists {
			return nil, fmt.Errorf("%w: duplicate endpoint after trimming %q", ErrInvalidDSPEndpointMap, endpoint)
		}
		clean[endpoint] = domain
	}

	ordered := orderedEndpointsFromMap(clean)
	var nativeMask filter.NativeFieldMask
	for i := range ordered {
		domain := ordered[i].Domain
		ordered[i].client = s.registry.Client(domain)
		if s.processor != nil {
			nativeMask |= s.processor.NativeMaskForDSPV25(DeletePrefix(domain))
		}
	}

	return &dspEndpointRuntime{mapa: clean, ordered: ordered, nativeMask: nativeMask}, nil
}

func orderedEndpointsFromMap(endpoints DSPEndpointMap) []DSPEndpointV25 {
	if len(endpoints) == 0 {
		return nil
	}
	ordered := make([]DSPEndpointV25, 0, len(endpoints))
	for endpoint, domain := range endpoints {
		ordered = append(ordered, DSPEndpointV25{Endpoint: endpoint, Domain: domain})
	}
	sortDSPEndpoints(ordered)
	return ordered
}

func (s *DSPEndpointStore) runtimeSnapshot() *dspEndpointRuntime {
	if s == nil {
		return &dspEndpointRuntime{mapa: DSPEndpointMap{}}
	}
	runtime, _ := s.runtime.Load().(*dspEndpointRuntime)
	if runtime == nil {
		return &dspEndpointRuntime{mapa: DSPEndpointMap{}}
	}
	return runtime
}

func (s *DSPEndpointStore) Snapshot() DSPEndpointMap {
	return cloneDSPEndpointMap(s.runtimeSnapshot().mapa)
}

func (s *DSPEndpointStore) ReadRaw() (DSPEndpointMap, error) {
	if s == nil || strings.TrimSpace(s.filename) == "" {
		return nil, errors.New("DSP endpoints map store is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return nil, err
	}
	return decodeDSPEndpointMap(data)
}

func (s *DSPEndpointStore) Update(mapa DSPEndpointMap) error {
	if s == nil || strings.TrimSpace(s.filename) == "" {
		return errors.New("DSP endpoints map store is not configured")
	}
	if mapa == nil {
		mapa = DSPEndpointMap{}
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	next, err := s.buildRuntime(mapa)
	if err != nil {
		return fmt.Errorf("validate DSP endpoints map: %w", err)
	}
	fileData, err := json.MarshalIndent(next.mapa, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal DSP endpoints map: %w", err)
	}
	fileData = append(fileData, '\n')
	if err := atomicWriteDSPEndpointFile(s.filename, fileData, 0644); err != nil {
		return fmt.Errorf("write DSP endpoints map %s: %w", s.filename, err)
	}

	// The disk commit succeeded; one atomic Store publishes endpoints, order,
	// native mask and clients together. Readers can only observe old or new.
	s.runtime.Store(next)
	return nil
}

func atomicWriteDSPEndpointFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	tmp, err := os.CreateTemp(dir, ".dsp-endpoints-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filename)
}
