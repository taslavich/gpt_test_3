package filter

import (
	"testing"

	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

func ptrString(v string) *string { return &v }
func ptrInt32(v int32) *int32    { return &v }

func processorWithDSPRules(t testing.TB, rules map[string][]RuleNode) *OptimizedFilterProcessor {
	t.Helper()
	manager := NewRuleManager()
	for dsp, nodes := range rules {
		roots := make([]*CompiledRuleNode, 0, len(nodes))
		all := make([]*FilterRule, 0)
		for _, node := range nodes {
			compiled, err := compileRuleNode(node)
			if err != nil {
				t.Fatalf("compile rule for %s: %v", dsp, err)
			}
			roots = append(roots, compiled)
			collectAllRules(compiled, &all)
		}
		manager.SetDSPRules(dsp+"|v25", roots, all)
	}
	return NewOptimizedFilterProcessor(manager)
}

func intRule(field FieldType, condition ConditionType, value string) RuleNode {
	return RuleNode{Field: field, Condition: condition, ValueType: ValueTypeInt, Value: []byte(value)}
}

func stringRule(field FieldType, condition ConditionType, value string) RuleNode {
	return RuleNode{Field: field, Condition: condition, ValueType: ValueTypeString, Value: []byte(value)}
}

func existsRule(field FieldType, valueType ValueType) RuleNode {
	return RuleNode{Field: field, Condition: ConditionExists, ValueType: valueType, Value: []byte(`""`)}
}

func nativeRequest(raw string) *ortb.BidRequest {
	return &ortb.BidRequest{Imp: []*ortb.Imp{{
		Id: ptrString("imp-1"),
		Native: &ortb.Native{
			Request: ptrString(raw),
			Ver:     ptrString("1.2"),
		},
	}}}
}

func TestV25NativeRequestAndVerDoNotParseJSON(t *testing.T) {
	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"dsp_native": {
			existsRule(FieldNativeRequest, ValueTypeString),
			stringRule(FieldNativeVer, ConditionEqual, `"1.2"`),
		},
	})
	ctx := NewV25RequestContext(nativeRequest(`{"assets":[{"id":1}]}`), constants.NAT, 0)
	if !processor.ProcessRequestContextForDSPV25("dsp_native", ctx).Allowed {
		t.Fatal("request/ver-only native rules must pass")
	}
	if ctx.nativeParseCount != 0 {
		t.Fatalf("native JSON parses=%d want 0", ctx.nativeParseCount)
	}
}

func TestV25NativeNestedFieldsParseOnceAcrossDSPs(t *testing.T) {
	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"dsp_a": {intRule(FieldNativeAssetID, ConditionEqual, `101`)},
		"dsp_b": {intRule(FieldNativeImgWMin, ConditionGreaterEqual, `300`)},
	})
	mask := processor.NativeMaskForDSPV25("dsp_a") | processor.NativeMaskForDSPV25("dsp_b")
	ctx := NewV25RequestContext(nativeRequest(`{
		"native":{"assets":[
			{"id":101,"required":1,"title":{"len":60}},
			{"id":102,"img":{"type":3,"w":1200,"h":627,"wmin":300,"hmin":250}},
			{"id":103,"data":{"type":2,"len":90}}
		]}
	}`), constants.NAT, mask)

	if !processor.ProcessRequestContextForDSPV25("dsp_a", ctx).Allowed {
		t.Fatal("asset id rule must pass")
	}
	if !processor.ProcessRequestContextForDSPV25("dsp_b", ctx).Allowed {
		t.Fatal("wmin rule must pass")
	}
	if ctx.nativeParseCount != 1 {
		t.Fatalf("native JSON parses=%d want exactly 1", ctx.nativeParseCount)
	}
}

func TestV25NestedNativeRuleOnBannerFailsWithoutParsing(t *testing.T) {
	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"dsp_native": {intRule(FieldNativeAssetID, ConditionEqual, `101`)},
	})
	req := &ortb.BidRequest{Imp: []*ortb.Imp{{
		Id: ptrString("imp-1"),
		Banner: &ortb.Banner{
			W: ptrInt32(300), H: ptrInt32(250),
		},
	}}}
	ctx := NewV25RequestContext(req, constants.BAN, processor.NativeMaskForDSPV25("dsp_native"))
	if processor.ProcessRequestContextForDSPV25("dsp_native", ctx).Allowed {
		t.Fatal("nested native rule must fail for BAN")
	}
	if ctx.nativeParseCount != 0 {
		t.Fatalf("native JSON parses=%d want 0", ctx.nativeParseCount)
	}
}

func TestV25NativeNumericPresence(t *testing.T) {
	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"has_required": {existsRule(FieldNativeRequired, ValueTypeInt)},
		"missing_len":  {existsRule(FieldNativeDataLen, ValueTypeInt)},
	})
	mask := processor.NativeMaskForDSPV25("has_required") | processor.NativeMaskForDSPV25("missing_len")
	ctx := NewV25RequestContext(nativeRequest(`{"assets":[{"id":1,"required":0,"data":{"type":2}}]}`), constants.NAT, mask)

	if !processor.ProcessRequestContextForDSPV25("has_required", ctx).Allowed {
		t.Fatal("required=0 is structurally present and must satisfy exists")
	}
	if processor.ProcessRequestContextForDSPV25("missing_len", ctx).Allowed {
		t.Fatal("missing data.len must not satisfy exists")
	}
	if ctx.nativeParseCount != 1 {
		t.Fatalf("native JSON parses=%d want exactly 1", ctx.nativeParseCount)
	}
}

func TestV25BannerFormatKeepsWidthHeightPairCorrelation(t *testing.T) {
	req := &ortb.BidRequest{Imp: []*ortb.Imp{{
		Id: ptrString("imp-1"),
		Banner: &ortb.Banner{Format: []*ortb.Format{
			{W: ptrInt32(300), H: ptrInt32(250)},
			{W: ptrInt32(728), H: ptrInt32(90)},
		}},
	}}}

	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"bad_pair": {
			intRule(FieldBannerFormatW, ConditionEqual, `300`),
			intRule(FieldBannerFormatH, ConditionEqual, `90`),
		},
		"good_pair": {
			intRule(FieldBannerFormatW, ConditionEqual, `300`),
			intRule(FieldBannerFormatH, ConditionEqual, `250`),
		},
	})

	badCtx := NewV25RequestContext(req, constants.BAN, 0)
	if processor.ProcessRequestContextForDSPV25("bad_pair", badCtx).Allowed {
		t.Fatal("W=300 and H=90 come from different format entries and must not match")
	}
	goodCtx := NewV25RequestContext(req, constants.BAN, 0)
	if !processor.ProcessRequestContextForDSPV25("good_pair", goodCtx).Allowed {
		t.Fatal("300x250 is a real banner.format pair and must match")
	}
}

func TestV25BannerMimesUsesAnyPositiveMatch(t *testing.T) {
	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"dsp_banner": {stringRule(FieldBannerMimes, ConditionEqual, `"image/png"`)},
	})
	req := &ortb.BidRequest{Imp: []*ortb.Imp{{Banner: &ortb.Banner{Mimes: []string{"image/jpeg", "image/png"}}}}}
	ctx := NewV25RequestContext(req, constants.BAN, 0)
	if !processor.ProcessRequestContextForDSPV25("dsp_banner", ctx).Allowed {
		t.Fatal("one matching banner mime must satisfy equal")
	}
}

func BenchmarkV25DSPFilterNativeNoJSONParse(b *testing.B) {
	processor := processorWithDSPRules(b, map[string][]RuleNode{
		"dsp_native": {existsRule(FieldNativeRequest, ValueTypeString)},
	})
	req := nativeRequest(`{"assets":[{"id":101,"img":{"wmin":300,"hmin":250}}]}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewV25RequestContext(req, constants.NAT, 0)
		if !processor.ProcessRequestContextForDSPV25("dsp_native", ctx).Allowed {
			b.Fatal("unexpected reject")
		}
	}
}

func BenchmarkV25DSPFilterNativeNestedSharedParse(b *testing.B) {
	processor := processorWithDSPRules(b, map[string][]RuleNode{
		"dsp_a": {intRule(FieldNativeAssetID, ConditionEqual, `101`)},
		"dsp_b": {intRule(FieldNativeImgWMin, ConditionGreaterEqual, `300`)},
	})
	mask := processor.NativeMaskForDSPV25("dsp_a") | processor.NativeMaskForDSPV25("dsp_b")
	req := nativeRequest(`{"native":{"assets":[{"id":101,"required":1,"img":{"type":3,"w":1200,"h":627,"wmin":300,"hmin":250}}]}}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewV25RequestContext(req, constants.NAT, mask)
		if !processor.ProcessRequestContextForDSPV25("dsp_a", ctx).Allowed || !processor.ProcessRequestContextForDSPV25("dsp_b", ctx).Allowed {
			b.Fatal("unexpected reject")
		}
	}
}

func TestLegacyCommonFieldSemanticsRemainUnchanged(t *testing.T) {
	legacy := NewStatelessV25BidRequestExtractor()
	req := &ortb.BidRequest{}

	missingSite := legacy.ExtractFieldValue(FieldSiteID, req)
	if (StringCondition{cond: ConditionExists}).Compare(missingSite) {
		t.Fatal("legacy missing string must still fail exists")
	}
	if !(StringCondition{cond: ConditionNotEqual, value: "some-site"}).Compare(missingSite) {
		t.Fatal("legacy missing string must preserve not_equal behavior")
	}

	missingAT := legacy.ExtractFieldValue(FieldAuctionType, req)
	if !(IntCondition{cond: ConditionExists}).Compare(missingAT) {
		t.Fatal("legacy missing numeric field must preserve exists behavior")
	}
}

func TestV25MissingFormatFieldFailsEvenNegativeCondition(t *testing.T) {
	processor := processorWithDSPRules(t, map[string][]RuleNode{
		"dsp_banner": {intRule(FieldBannerW, ConditionNotEqual, `300`)},
	})
	req := &ortb.BidRequest{Imp: []*ortb.Imp{{Native: &ortb.Native{Request: ptrString(`{"assets":[]}`)}}}}
	ctx := NewV25RequestContext(req, constants.NAT, 0)
	if processor.ProcessRequestContextForDSPV25("dsp_banner", ctx).Allowed {
		t.Fatal("missing banner.w must fail not_equal instead of matching a synthetic zero")
	}
}
