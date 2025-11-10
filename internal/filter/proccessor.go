package filter

import (
	"fmt"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

type OptimizedFilterProcessor struct {
	ruleManager *RuleManager
	// Stateless экстракторы (создаются один раз)
	v25ReqExtractor  *StatelessV25BidRequestExtractor
	v25RespExtractor *StatelessV25BidResponseExtractor
}

func NewOptimizedFilterProcessor(ruleManager *RuleManager) *OptimizedFilterProcessor {
	return &OptimizedFilterProcessor{
		ruleManager:      ruleManager,
		v25ReqExtractor:  NewStatelessV25BidRequestExtractor(),
		v25RespExtractor: NewStatelessV25BidResponseExtractor(),
	}
}

// processor.go

// Рекурсивная оценка узла дерева правил
func (fp *OptimizedFilterProcessor) evaluateRuleNode(node *CompiledRuleNode, extractor BidRequestExtractor, req interface{}) bool {
	switch node.Operator {
	case "leaf":
		// Листовой узел - простое правило
		fieldValue := extractor.ExtractFieldValue(node.Rule.Field, req)
		return node.Rule.Value.Compare(fieldValue)

	case "and":
		// AND группа - все дочерние узлы должны быть true
		for _, child := range node.Children {
			if !fp.evaluateRuleNode(child, extractor, req) {
				return false
			}
		}
		return true

	case "or":
		// OR группа - хотя бы один дочерний узел должен быть true
		for _, child := range node.Children {
			if fp.evaluateRuleNode(child, extractor, req) {
				return true
			}
		}
		return false

	default:
		return false
	}
}

// Оптимизированный метод с рекурсивной проверкой
func (fp *OptimizedFilterProcessor) processRequestForDSPOptimized(dspURL string, extractor BidRequestExtractor, req interface{}) *FilterResult {
	ruleSet := fp.ruleManager.GetCompiledRulesForDSP(dspURL)
	if ruleSet == nil || len(ruleSet.rootNodes) == 0 {
		return &FilterResult{Allowed: true}
	}

	// Все корневые узлы работают по AND
	for _, rootNode := range ruleSet.rootNodes {
		if !fp.evaluateRuleNode(rootNode, extractor, req) {
			return &FilterResult{Allowed: false}
		}
	}

	return &FilterResult{Allowed: true}
}

// ProcessResponseForSPPV25 обрабатывает BidResponse v2.5 для SPP
func (fp *OptimizedFilterProcessor) ProcessResponseForSPPV25(sppURL string, resp *ortb_V2_5.BidResponse) *FilterResult {
	if resp == nil {
		return &FilterResult{Allowed: false}
	}
	versionedSPPID := fmt.Sprintf("%s|v25", sppURL)
	return fp.processResponseForSPPOptimized(versionedSPPID, fp.v25RespExtractor, resp)
}

// Оптимизированный метод с рекурсивной проверкой для SPP
func (fp *OptimizedFilterProcessor) processResponseForSPPOptimized(sppURL string, extractor BidResponseExtractor, resp interface{}) *FilterResult {
	ruleSet := fp.ruleManager.GetCompiledRulesForSPP(sppURL)
	autoRules := GetAutoRulesForSPP()

	if ruleSet == nil && len(autoRules) == 0 {
		return &FilterResult{Allowed: true}
	}

	// Если есть авто-правила, создаем для них корневой узел
	if len(autoRules) > 0 {
		autoRootNode := &CompiledRuleNode{
			Operator: "and",
			Children: make([]*CompiledRuleNode, 0, len(autoRules)),
		}

		for _, rule := range autoRules {
			autoRootNode.Children = append(autoRootNode.Children, &CompiledRuleNode{
				Rule:     rule,
				Operator: "leaf",
			})
		}

		// Если есть обычные правила, объединяем с авто-правилами через AND
		if ruleSet != nil {
			combinedRoot := &CompiledRuleNode{
				Operator: "and",
				Children: append([]*CompiledRuleNode{autoRootNode}, ruleSet.rootNodes...),
			}

			if !fp.evaluateRuleNode(combinedRoot, extractor, resp) {
				return &FilterResult{Allowed: false}
			}
		} else {
			// Только авто-правила
			if !fp.evaluateRuleNode(autoRootNode, extractor, resp) {
				return &FilterResult{Allowed: false}
			}
		}
	} else {
		// Только обычные правила
		for _, rootNode := range ruleSet.rootNodes {
			if !fp.evaluateRuleNode(rootNode, extractor, resp) {
				return &FilterResult{Allowed: false}
			}
		}
	}

	return &FilterResult{Allowed: true}
}

// ProcessRequestForDSPV25 обрабатывает BidRequest v2.5 для DSP
func (fp *OptimizedFilterProcessor) ProcessRequestForDSPV25(dspURL string, req *ortb_V2_5.BidRequest) *FilterResult {
	if req == nil {
		return &FilterResult{Allowed: false}
	}
	versionedDSPID := fmt.Sprintf("%s|v25", dspURL)
	return fp.processRequestForDSPOptimized(versionedDSPID, fp.v25ReqExtractor, req)
}
