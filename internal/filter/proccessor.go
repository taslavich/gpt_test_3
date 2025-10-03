package filter

import (
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_4"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

type OptimizedFilterProcessor struct {
	ruleManager *RuleManager
	// Stateless экстракторы (создаются один раз)
	v24ReqExtractor  *StatelessV24BidRequestExtractor
	v25ReqExtractor  *StatelessV25BidRequestExtractor
	v24RespExtractor *StatelessV24BidResponseExtractor
	v25RespExtractor *StatelessV25BidResponseExtractor
}

func NewOptimizedFilterProcessor(ruleManager *RuleManager) *OptimizedFilterProcessor {
	return &OptimizedFilterProcessor{
		ruleManager:      ruleManager,
		v24ReqExtractor:  NewStatelessV24BidRequestExtractor(),
		v25ReqExtractor:  NewStatelessV25BidRequestExtractor(),
		v24RespExtractor: NewStatelessV24BidResponseExtractor(),
		v25RespExtractor: NewStatelessV25BidResponseExtractor(),
	}
}

// ProcessRequestForDSPV24 обрабатывает BidRequest v2.4 для DSP
func (fp *OptimizedFilterProcessor) ProcessRequestForDSPV24(dspURL string, req *ortb_V2_4.BidRequest) *FilterResult {
	if req == nil {
		return &FilterResult{Allowed: false}
	}
	return fp.processRequestForDSPOptimized(dspURL, fp.v24ReqExtractor, req)
}

// ProcessRequestForDSPV25 обрабатывает BidRequest v2.5 для DSP
func (fp *OptimizedFilterProcessor) ProcessRequestForDSPV25(dspURL string, req *ortb_V2_5.BidRequest) *FilterResult {
	if req == nil {
		return &FilterResult{Allowed: false}
	}
	return fp.processRequestForDSPOptimized(dspURL, fp.v25ReqExtractor, req)
}

// ProcessResponseForSPPV24 обрабатывает BidResponse v2.4 для SPP
func (fp *OptimizedFilterProcessor) ProcessResponseForSPPV24(sppURL string, resp *ortb_V2_4.BidResponse) *FilterResult {
	if resp == nil {
		return &FilterResult{Allowed: false}
	}
	return fp.processResponseForSPPOptimized(sppURL, fp.v24RespExtractor, resp)
}

// ProcessResponseForSPPV25 обрабатывает BidResponse v2.5 для SPP
func (fp *OptimizedFilterProcessor) ProcessResponseForSPPV25(sppURL string, resp *ortb_V2_5.BidResponse) *FilterResult {
	if resp == nil {
		return &FilterResult{Allowed: false}
	}
	return fp.processResponseForSPPOptimized(sppURL, fp.v25RespExtractor, resp)
}

// Оптимизированный метод с bulk extraction для DSP
func (fp *OptimizedFilterProcessor) processRequestForDSPOptimized(dspURL string, extractor BidRequestExtractor, req interface{}) *FilterResult {
	ruleSet := fp.ruleManager.GetCompiledRulesForDSP(dspURL)
	if ruleSet == nil || len(ruleSet.rules) == 0 {
		return &FilterResult{Allowed: true}
	}

	// Bulk extraction: группируем правила по полям для избежания повторного извлечения
	fieldRules := make(map[FieldType][]*FilterRule)
	for _, rule := range ruleSet.rules {
		fieldRules[rule.Field] = append(fieldRules[rule.Field], rule)
	}

	// Проверяем правила группами по полям
	for field, rules := range fieldRules {
		fieldValue := extractor.ExtractFieldValue(field, req)

		for _, rule := range rules {
			if !rule.Value.Compare(fieldValue) {
				return &FilterResult{Allowed: false}
			}
		}
	}

	return &FilterResult{Allowed: true}
}

// Оптимизированный метод с bulk extraction для SPP
func (fp *OptimizedFilterProcessor) processResponseForSPPOptimized(sppURL string, extractor BidResponseExtractor, resp interface{}) *FilterResult {
	ruleSet := fp.ruleManager.GetCompiledRulesForSPP(sppURL)
	autoRules := GetAutoRulesForSPP()

	if ruleSet == nil && len(autoRules) == 0 {
		return &FilterResult{Allowed: true}
	}

	// Собираем все правила
	allRules := make([]*FilterRule, 0)
	if ruleSet != nil {
		allRules = append(allRules, ruleSet.rules...)
	}
	allRules = append(allRules, autoRules...)

	// Bulk extraction: группируем правила по полям
	fieldRules := make(map[FieldType][]*FilterRule)
	for _, rule := range allRules {
		fieldRules[rule.Field] = append(fieldRules[rule.Field], rule)
	}

	// Проверяем правила группами по полям
	for field, rules := range fieldRules {
		fieldValue := extractor.ExtractFieldValue(field, resp)

		for _, rule := range rules {
			if !rule.Value.Compare(fieldValue) {
				return &FilterResult{Allowed: false}
			}
		}
	}

	return &FilterResult{Allowed: true}
}
