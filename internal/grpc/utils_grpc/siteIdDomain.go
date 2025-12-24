package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strings"
	"sync/atomic"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

var (
	genID      atomic.Uint64
	generators [256]*rand.PCG
)

func init() {
	seed := uint64(time.Now().UnixNano())
	for i := range generators {
		generators[i] = rand.NewPCG(seed, uint64(i+1)*0x9e3779b97f4a7c15)
	}
}

type SiteIdsAndDomains struct {
	siteIdDomainCommonByIds cmap.ConcurrentMap[string, string]
	siteIdDomainDeltaByIds  cmap.ConcurrentMap[string, string]

	domainCommonSet cmap.ConcurrentMap[string, struct{}]
	domainDeltaSet  cmap.ConcurrentMap[string, struct{}]

	level1Domains  map[uint]string
	level23Domains map[uint]string

	filenameSiteIdsDomains string
	filenameDomainLevel1   string
	filenameDomainLevel23  string
}

func NewSiteIdsAndDomains(filenameSiteIdsDomains, filenameDomainLevel1, filenameDomainLevel23 string) (*SiteIdsAndDomains, error) {
	siteIdCommon, domainCommonSet, err := loadSiteIdsDomains(filenameSiteIdsDomains)
	if err != nil {
		return nil, fmt.Errorf("Cannot loadSiteIdsDomains: %v", err)
	}

	level1Domains, err := loadIndexedDomainsFromJSON(filenameDomainLevel1)
	if err != nil {
		return nil, fmt.Errorf("Cannot loadIndexedDomainsFromJSON in filenameDomainLevel1: %v", err)
	}

	level23Domains, err := loadIndexedDomainsFromJSON(filenameDomainLevel23)
	if err != nil {
		return nil, fmt.Errorf("Cannot loadIndexedDomainsFromJSON in filenameDomainLevel23: %v", err)
	}

	return &SiteIdsAndDomains{
		siteIdDomainCommonByIds: siteIdCommon,
		siteIdDomainDeltaByIds:  cmap.New[string](),
		domainCommonSet:         domainCommonSet,
		domainDeltaSet:          cmap.New[struct{}](),

		level1Domains:  level1Domains,
		level23Domains: level23Domains,

		filenameSiteIdsDomains: filenameSiteIdsDomains,
		filenameDomainLevel1:   filenameDomainLevel1,
		filenameDomainLevel23:  filenameDomainLevel23,
	}, nil
}

func loadSiteIdsDomains(filename string) (cmap.ConcurrentMap[string, string], cmap.ConcurrentMap[string, struct{}], error) {
	f, err := os.Open(filename)
	if err != nil {
		return cmap.New[string](), cmap.New[struct{}](), fmt.Errorf("Cannot open file %s in ReadSiteIdDomainFromFile: %v", filename, err)
	}
	defer f.Close()

	siteIdCommon := cmap.New[string]()
	domainSet := cmap.New[struct{}]()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		sepIndex := strings.IndexByte(line, '|')
		if sepIndex < 0 {
			continue
		}

		key := line[:sepIndex]
		value := line[sepIndex+1:]
		siteIdCommon.Set(key, value)
		domainSet.Set(value, struct{}{})
	}

	if err := scanner.Err(); err != nil {
		return cmap.New[string](), cmap.New[struct{}](), err
	}

	return siteIdCommon, domainSet, nil
}

func loadIndexedDomainsFromJSON(filename string) (map[uint]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var domains map[uint]string
	if err := json.Unmarshal(data, &domains); err != nil {
		return nil, fmt.Errorf("Invalid JSON format in %s: %v", filename, err)
	}

	return domains, nil
}

func (s *SiteIdsAndDomains) WriteSiteIdDomainToTheFile() error {
	f, err := os.OpenFile(s.filenameSiteIdsDomains, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("Cannot open file %s in WriteSiteIdDomainToTheFile: %v", s.filenameSiteIdsDomains, err)
	}
	defer f.Close()

	writer := bufio.NewWriterSize(f, 16*1024*1024)

	deltaBufferByIds := s.siteIdDomainDeltaByIds.Items()
	deltaBufferSet := s.domainDeltaSet.Items()

	for k, v := range deltaBufferByIds {
		writer.WriteString(k)
		writer.WriteByte('|')
		writer.WriteString(v)
		writer.WriteByte('\n')
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("Cannot flush file %s in WriteSiteIdDomainToTheFile: %v", s.filenameSiteIdsDomains, err)
	}

	s.siteIdDomainCommonByIds.MSet(deltaBufferByIds)
	s.domainCommonSet.MSet(deltaBufferSet)

	for key := range deltaBufferByIds {
		s.siteIdDomainDeltaByIds.Remove(key)
	}

	for key := range deltaBufferSet {
		s.domainDeltaSet.Remove(key)
	}

	log.Println("FINISHED WriteSiteIdDomainToTheFile")
	return nil
}

func (s *SiteIdsAndDomains) GenerateDomain(siteId string) string {
	if val, ok := s.siteIdDomainDeltaByIds.Get(siteId); ok {
		return val
	}

	if val, ok := s.siteIdDomainCommonByIds.Get(siteId); ok {
		return val
	}

	id := genID.Add(1)
	rng := generators[id&255]
	n2 := rng.Uint64()
	n3 := rng.Uint64()
	n4 := rng.Uint64()

	var response string

	for true {
		level1Index := 1 + uint(n2%63)
		level2Index := 1 + uint(n3%10000)
		level3Index := 1 + uint(n4%10000)

		level1 := s.level1Domains[level1Index]
		level2 := s.level23Domains[level2Index]
		level3 := s.level23Domains[level3Index]
		response = fmt.Sprintf("%s.%s.%s", level3, level2, level1)
		if _, ok := s.domainDeltaSet.Get(response); ok {
			continue
		}

		if _, ok := s.domainCommonSet.Get(response); ok {
			continue
		}

		break
	}

	s.siteIdDomainDeltaByIds.Set(siteId, response)
	s.domainDeltaSet.Set(response, struct{}{})

	return response
}
