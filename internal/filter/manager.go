package filter

import (
	"sync"
)

type CompiledRuleNode struct {
	Rule     *FilterRule         `json:"-"`
	Operator string              `json:"operator"` // "and", "or", "leaf"
	Children []*CompiledRuleNode `json:"children,omitempty"`
}

type CompiledRuleSet struct {
	rootNodes      []*CompiledRuleNode
	requiredFields []FieldType
	fieldRules     map[FieldType][]*FilterRule
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

func (rm *RuleManager) SetDSPRules(dspID string, rootNodes []*CompiledRuleNode, allRules []*FilterRule) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.dspRules[dspID] = rm.compileRules(rootNodes, allRules)
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
			ID:        "auto_adm_exists",
			Field:     FieldBidAdm,
			Condition: ConditionExists,
			Value:     StringCondition{cond: ConditionExists, value: ""},
		},
	}
}

func (rm *RuleManager) compileRules(rootNodes []*CompiledRuleNode, allRules []*FilterRule) *CompiledRuleSet {
	fieldsSet := make(map[FieldType]struct{}, len(allRules))
	fieldRules := make(map[FieldType][]*FilterRule)

	for _, rule := range allRules {
		fieldsSet[rule.Field] = struct{}{}
		fieldRules[rule.Field] = append(fieldRules[rule.Field], rule)
	}

	requiredFields := make([]FieldType, 0, len(fieldsSet))
	for field := range fieldsSet {
		requiredFields = append(requiredFields, field)
	}

	return &CompiledRuleSet{
		rootNodes:      rootNodes,
		requiredFields: requiredFields,
		fieldRules:     fieldRules,
	}
}

// Добавляем метод для компиляции старых правил (для обратной совместимости)
func (rm *RuleManager) compileSimpleRules(rules map[string]*FilterRule) *CompiledRuleSet {
	ruleSlice := make([]*FilterRule, 0, len(rules))
	for _, rule := range rules {
		ruleSlice = append(ruleSlice, rule)
	}

	// Создаем простой корневой узел AND для всех правил
	rootNode := &CompiledRuleNode{
		Operator: "and",
		Children: make([]*CompiledRuleNode, 0, len(ruleSlice)),
	}

	for _, rule := range ruleSlice {
		rootNode.Children = append(rootNode.Children, &CompiledRuleNode{
			Rule:     rule,
			Operator: "leaf",
		})
	}

	rootNodes := []*CompiledRuleNode{rootNode}

	return rm.compileRules(rootNodes, ruleSlice)
}

func (rm *RuleManager) SetSPPRules(sppID string, rootNodes []*CompiledRuleNode, allRules []*FilterRule) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.sppRules[sppID] = rm.compileRules(rootNodes, allRules)
}
