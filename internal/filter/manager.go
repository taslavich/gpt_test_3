package filter

import (
	"sync"
)

type CompiledRuleSet struct {
	rules          []*FilterRule
	requiredFields []FieldType
	// Для bulk optimization - группировка правил по полям
	fieldRules map[FieldType][]*FilterRule
}

type RuleManager struct {
	dspRules map[string]*CompiledRuleSet
	sppRules map[string]*CompiledRuleSet
	mu       sync.RWMutex
}

func NewRuleManager() *RuleManager {
	return &RuleManager{
		dspRules: make(map[string]*CompiledRuleSet),
		sppRules: make(map[string]*CompiledRuleSet),
	}
}

func (rm *RuleManager) compileRules(rules map[string]*FilterRule) *CompiledRuleSet {
	ruleSlice := make([]*FilterRule, 0, len(rules))
	fieldsSet := make(map[FieldType]struct{}, len(rules))
	fieldRules := make(map[FieldType][]*FilterRule)

	for _, rule := range rules {
		ruleSlice = append(ruleSlice, rule)
		fieldsSet[rule.Field] = struct{}{}
		fieldRules[rule.Field] = append(fieldRules[rule.Field], rule)
	}

	requiredFields := make([]FieldType, 0, len(fieldsSet))
	for field := range fieldsSet {
		requiredFields = append(requiredFields, field)
	}

	return &CompiledRuleSet{
		rules:          ruleSlice,
		requiredFields: requiredFields,
		fieldRules:     fieldRules,
	}
}

// GetFieldRulesForDSP возвращает правила сгруппированные по полям для bulk extraction
func (rm *RuleManager) GetFieldRulesForDSP(dspID string) map[FieldType][]*FilterRule {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if ruleSet := rm.dspRules[dspID]; ruleSet != nil {
		return ruleSet.fieldRules
	}
	return nil
}

// GetFieldRulesForSPP возвращает правила сгруппированные по полям для bulk extraction
func (rm *RuleManager) GetFieldRulesForSPP(sppID string) map[FieldType][]*FilterRule {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if ruleSet := rm.sppRules[sppID]; ruleSet != nil {
		return ruleSet.fieldRules
	}
	return nil
}

// Остальные методы остаются без изменений...
func (rm *RuleManager) SetDSPRules(dspID string, rules map[string]*FilterRule) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dspRules[dspID] = rm.compileRules(rules)
}

func (rm *RuleManager) SetSPPRules(sppID string, rules map[string]*FilterRule) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.sppRules[sppID] = rm.compileRules(rules)
}

func (rm *RuleManager) GetCompiledRulesForDSP(dspID string) *CompiledRuleSet {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.dspRules[dspID]
}

func (rm *RuleManager) GetCompiledRulesForSPP(sppID string) *CompiledRuleSet {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.sppRules[sppID]
}

func (rm *RuleManager) ClearAllDSPRules() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dspRules = make(map[string]*CompiledRuleSet)
}

func (rm *RuleManager) ClearAllSPPRules() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.sppRules = make(map[string]*CompiledRuleSet)
}

// Статические авто-правила (создаются один раз)
func GetAutoRulesForSPP() []*FilterRule {
	return []*FilterRule{
		{
			ID:        "auto_nurl_exists",
			Field:     FieldBidNurl,
			Condition: ConditionExists,
			Value:     StringCondition{cond: ConditionExists, value: ""},
		},
		{
			ID:        "auto_burl_exists",
			Field:     FieldBidBurl,
			Condition: ConditionExists,
			Value:     StringCondition{cond: ConditionExists, value: ""},
		},
	}
}
