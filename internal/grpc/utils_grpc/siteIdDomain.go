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

	"golang.org/x/exp/maps"
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
	siteIdDomainCommonByIds map[string]string
	siteIdDomainDeltaByIds  map[string]string

	domainCommonSet map[string]struct{}
	domainDeltaSet  map[string]struct{}

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
		siteIdDomainDeltaByIds:  make(map[string]string),
		domainCommonSet:         domainCommonSet,
		domainDeltaSet:          map[string]struct{}{},

		level1Domains:  level1Domains,
		level23Domains: level23Domains,

		filenameSiteIdsDomains: filenameSiteIdsDomains,
		filenameDomainLevel1:   filenameDomainLevel1,
		filenameDomainLevel23:  filenameDomainLevel23,
	}, nil
}

func loadSiteIdsDomains(filename string) (map[string]string, map[string]struct{}, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, fmt.Errorf("Cannot open file %s in ReadSiteIdDomainFromFile: %v", filename, err)
	}
	defer f.Close()

	siteIdCommon := make(map[string]string)
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
		siteIdCommon[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	return siteIdCommon, ValuesSet(siteIdCommon), nil
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

	deltaBufferByIds := maps.Clone(s.siteIdDomainDeltaByIds)
	deltaBufferSet := maps.Clone(s.domainDeltaSet)

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

	maps.Copy(s.siteIdDomainCommonByIds, deltaBufferByIds)
	maps.Copy(s.domainCommonSet, deltaBufferSet)

	for key := range deltaBufferByIds {
		delete(s.siteIdDomainDeltaByIds, key)
	}

	for key := range deltaBufferSet {
		delete(s.domainDeltaSet, key)
	}

	log.Println("FINISHED WriteSiteIdDomainToTheFile")
	return nil
}

func (s *SiteIdsAndDomains) GenerateDomain(siteId string) string {
	if val, ok := s.siteIdDomainDeltaByIds[siteId]; ok {
		return val
	}

	if val, ok := s.siteIdDomainCommonByIds[siteId]; ok {
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
		if _, ok := s.domainDeltaSet[response]; ok {
			continue
		}

		if _, ok := s.domainCommonSet[response]; ok {
			continue
		}

		break
	}

	return response
}
