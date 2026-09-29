package sppAdapterWeb

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
)

// FeedMap maps the public feed UUID supplied by an SSP to the SSP domain used
// by the existing auction/statistics path.
type FeedMap map[string]string

// FeedStore follows the same control-plane model used by BidEngine stores:
// the JSON file is the editable source on disk, while runtime is an immutable
// snapshot swapped atomically by PUT. Manual file edits are picked up on the
// next process restart.
type FeedStore struct {
	updateMu sync.Mutex
	filename string
	runtime  atomic.Value // FeedMap
}

func NewFeedStore(filename string) (*FeedStore, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, errors.New("SSP feeds map filename is empty")
	}

	store := &FeedStore{filename: filename}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read SSP feeds map %s: %w", filename, err)
	}
	mapa, err := decodeFeedMap(data)
	if err != nil {
		return nil, fmt.Errorf("parse SSP feeds map %s: %w", filename, err)
	}
	store.runtime.Store(mapa)
	return store, nil
}

func decodeFeedMap(data []byte) (FeedMap, error) {
	if strings.TrimSpace(string(data)) == "" {
		return FeedMap{}, nil
	}
	var mapa FeedMap
	if err := json.Unmarshal(data, &mapa); err != nil {
		return nil, err
	}
	if mapa == nil {
		return FeedMap{}, nil
	}
	return mapa, nil
}

func cloneFeedMap(input FeedMap) FeedMap {
	output := make(FeedMap, len(input))
	for feed, domain := range input {
		output[feed] = domain
	}
	return output
}

func (s *FeedStore) Lookup(feed string) (string, bool) {
	if s == nil {
		return "", false
	}
	mapa, _ := s.runtime.Load().(FeedMap)
	domain, ok := mapa[feed]
	return domain, ok
}

func (s *FeedStore) Snapshot() FeedMap {
	if s == nil {
		return FeedMap{}
	}
	mapa, _ := s.runtime.Load().(FeedMap)
	return cloneFeedMap(mapa)
}

func (s *FeedStore) ReadRaw() (FeedMap, error) {
	if s == nil || strings.TrimSpace(s.filename) == "" {
		return nil, errors.New("SSP feeds map store is not configured")
	}
	data, err := os.ReadFile(s.filename)
	if err != nil {
		return nil, err
	}
	return decodeFeedMap(data)
}

func (s *FeedStore) Update(mapa FeedMap) error {
	if s == nil || strings.TrimSpace(s.filename) == "" {
		return errors.New("SSP feeds map store is not configured")
	}
	if mapa == nil {
		mapa = FeedMap{}
	}

	fileData, err := json.MarshalIndent(mapa, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal SSP feeds map: %w", err)
	}
	fileData = append(fileData, '\n')

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	if err := atomicWriteFeedFile(s.filename, fileData, 0644); err != nil {
		return fmt.Errorf("write SSP feeds map %s: %w", s.filename, err)
	}
	s.runtime.Store(cloneFeedMap(mapa))
	return nil
}

func atomicWriteFeedFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	tmp, err := os.CreateTemp(dir, ".ssp-feeds-*.tmp")
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

type FormatFeedRouteV25 struct {
	Adult      *FeedStore
	Mainstream *FeedStore
}

type FormatFeedRoutesV25 struct {
	POP FormatFeedRouteV25
	BAN FormatFeedRouteV25
	NAT FormatFeedRouteV25
	IPP FormatFeedRouteV25
	VID FormatFeedRouteV25
}

func normalizeFeedFormat(format string) string {
	switch strings.ToUpper(strings.TrimSpace(format)) {
	case constants.POP:
		return constants.POP
	case constants.BAN:
		return constants.BAN
	case constants.NAT:
		return constants.NAT
	case constants.IPP:
		return constants.IPP
	case constants.VID:
		return constants.VID
	default:
		return ""
	}
}

func requestedFeedFormat(format string) string {
	if strings.TrimSpace(format) == "" {
		return constants.POP
	}
	return normalizeFeedFormat(format)
}

func (r *FormatFeedRoutesV25) Select(format, typic string) *FeedStore {
	if r == nil {
		return nil
	}
	var route *FormatFeedRouteV25
	switch normalizeFeedFormat(format) {
	case constants.POP:
		route = &r.POP
	case constants.BAN:
		route = &r.BAN
	case constants.NAT:
		route = &r.NAT
	case constants.IPP:
		route = &r.IPP
	case constants.VID:
		route = &r.VID
	default:
		return nil
	}
	switch typic {
	case ADULT:
		return route.Adult
	case MAINSTREAM:
		return route.Mainstream
	default:
		return nil
	}
}
