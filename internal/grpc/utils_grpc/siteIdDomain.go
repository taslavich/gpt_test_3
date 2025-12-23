package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func WriteSiteIdDomainToTheFile(siteIdDomainCommon *map[string]string, siteIdDomainDelta map[string]string, filename string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("Cannot open file %s in WriteSiteIdDomainToTheFile: %v", filename, err)
	}
	defer f.Close()

	writer := bufio.NewWriterSize(f, 16*1024*1024)

	for k, v := range siteIdDomainDelta {
		writer.WriteString(k)
		writer.WriteByte('|')
		writer.WriteString(v)
		writer.WriteByte('\n')
	}

	return writer.Flush()
}

func ReadSiteIdDomainFromFile(filename string) (map[string]string, map[string]string, error) {
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

	return siteIdCommon, make(map[string]string), nil
}
