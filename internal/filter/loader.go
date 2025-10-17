package filter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fsnotify/fsnotify"
)

type FileRuleLoader struct {
	ruleManager    *RuleManager
	dspV24FilePath string
	dspV25FilePath string
	sppV24FilePath string
	sppV25FilePath string
	watcher        *fsnotify.Watcher
}

func NewFileRuleLoader(ruleManager *RuleManager, dspV24Path, dspV25Path, sppV24Path, sppV25Path string) *FileRuleLoader {
	return &FileRuleLoader{
		ruleManager:    ruleManager,
		dspV24FilePath: dspV24Path,
		dspV25FilePath: dspV25Path,
		sppV24FilePath: sppV24Path,
		sppV25FilePath: sppV25Path,
	}
}

func (fl *FileRuleLoader) LoadDSPRules() error {
	if err := fl.loadDSPRulesForVersion(fl.dspV25FilePath, "v25"); err != nil {
		return err
	}
	return nil
}

func (fl *FileRuleLoader) loadDSPRulesForVersion(filePath, version string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var config VersionedRuleConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	for dspID, dspSettings := range config.DSPs {
		var rules []RuleNode
		switch version {
		case "v25":
			rules = dspSettings.V25
		default:
			return fmt.Errorf("unknown version: %s", version)
		}

		rootNodes := make([]*CompiledRuleNode, 0, len(rules))
		allRules := make([]*FilterRule, 0)
		for _, ruleNode := range rules {
			rootNode, err := compileRuleNode(ruleNode)
			if err != nil {
				return fmt.Errorf("Error compiling rule node for DSP %s (%s): %v", dspID, version, err)
			}
			rootNodes = append(rootNodes, rootNode)
			collectAllRules(rootNode, &allRules)
		}
		versionedDSPID := fmt.Sprintf("%s|%s", dspID, version)
		fl.ruleManager.SetDSPRules(versionedDSPID, rootNodes, allRules)
	}
	return nil
}

func (fl *FileRuleLoader) LoadSPPRules() error {
	if err := fl.loadSPPRulesForVersion(fl.sppV25FilePath, "v25"); err != nil {
		return err
	}
	return nil
}

func (fl *FileRuleLoader) loadSPPRulesForVersion(filePath, version string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var config VersionedRuleConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	for sppID, sppSettings := range config.SPPs {
		var rules []RuleNode
		switch version {
		case "v25":
			rules = sppSettings.V25
		default:
			return fmt.Errorf("unknown version: %s", version)
		}

		rootNodes := make([]*CompiledRuleNode, 0, len(rules))
		allRules := make([]*FilterRule, 0)
		for _, ruleNode := range rules {
			rootNode, err := compileRuleNode(ruleNode)
			if err != nil {
				return fmt.Errorf("Error compiling rule node for SPP %s (%s): %v", sppID, version, err)
			}
			rootNodes = append(rootNodes, rootNode)
			collectAllRules(rootNode, &allRules)
		}
		versionedSPPID := fmt.Sprintf("%s|%s", sppID, version)
		fl.ruleManager.SetSPPRules(versionedSPPID, rootNodes, allRules)
	}
	return nil
}

func collectAllRules(node *CompiledRuleNode, rules *[]*FilterRule) {
	if node.Rule != nil {
		*rules = append(*rules, node.Rule)
	}
	for _, child := range node.Children {
		collectAllRules(child, rules)
	}
}

func (fl *FileRuleLoader) Close() error {
	if fl.watcher != nil {
		return fl.watcher.Close()
	}
	return nil
}
