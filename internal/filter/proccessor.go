package filter

import (
	"fmt"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

type OptimizedFilterProcessor struct {
	ruleManager *RuleManager
	// Stateless response extractor (created once). DSP request filtering uses a
	// request-local V25RequestContext for lazy format-specific extraction.
	v25RespExtractor *StatelessV25BidResponseExtractor
}

func NewOptimizedFilterProcessor(ruleManager *RuleManager) *OptimizedFilterProcessor {
	return &OptimizedFilterProcessor{
		ruleManager:      ruleManager,
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

	evaluateAll := func() bool {
		// Все корневые узлы работают по AND.
		for _, rootNode := range ruleSet.rootNodes {
			if !fp.evaluateRuleNode(rootNode, extractor, req) {
				return false
			}
		}
		return true
	}

	// banner.format is an array of size objects. If both width and height are
	// constrained, evaluate them against the same array element instead of
	// independently matching W from one size and H from another.
	if reqCtx, ok := extractor.(*V25RequestContext); ok && ruleSet.usesBannerFormatW && ruleSet.usesBannerFormatH && !reqCtx.bannerPairBound {
		return &FilterResult{Allowed: reqCtx.withBannerFormatPairs(evaluateAll)}
	}

	return &FilterResult{Allowed: evaluateAll()}
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

// NativeMaskForDSPV25 returns the precompiled set of nested native fields
// needed by one DSP. A DSP without rules returns zero.
func (fp *OptimizedFilterProcessor) NativeMaskForDSPV25(dspURL string) NativeFieldMask {
	if fp == nil || fp.ruleManager == nil {
		return 0
	}
	return fp.ruleManager.GetNativeMaskForDSP(fmt.Sprintf("%s|v25", dspURL))
}

// ProcessRequestContextForDSPV25 evaluates one DSP against a shared request
// context. Sharing the context is what guarantees a maximum of one native JSON
// parse across all DSPs for the request.
func (fp *OptimizedFilterProcessor) ProcessRequestContextForDSPV25(dspURL string, reqCtx *V25RequestContext) *FilterResult {
	if fp == nil || reqCtx == nil || reqCtx.request == nil {
		return &FilterResult{Allowed: false}
	}
	versionedDSPID := fmt.Sprintf("%s|v25", dspURL)
	return fp.processRequestForDSPOptimized(versionedDSPID, reqCtx, reqCtx)
}

// ProcessRequestForDSPV25 is kept for callers/tests that do not share a
// request context. Router uses ProcessRequestContextForDSPV25 instead.
func (fp *OptimizedFilterProcessor) ProcessRequestForDSPV25(dspURL string, req *ortb_V2_5.BidRequest) *FilterResult {
	if req == nil {
		return &FilterResult{Allowed: false}
	}
	mask := fp.NativeMaskForDSPV25(dspURL)
	reqCtx := NewV25RequestContext(req, inferV25Format(req), mask)
	return fp.ProcessRequestContextForDSPV25(dspURL, reqCtx)
}

func inferV25Format(req *ortb_V2_5.BidRequest) string {
	if req == nil {
		return ""
	}
	for _, imp := range req.GetImp() {
		if imp == nil {
			continue
		}
		if imp.GetNative() != nil {
			return "NAT"
		}
		if imp.GetBanner() != nil {
			return "BAN"
		}
	}
	return "POP"
}
