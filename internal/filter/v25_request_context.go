package filter

import (
	"strings"

	jsoniter "github.com/json-iterator/go"
	"gitlab.com/twinbid-exchange/RTB-exchange/internal/constants"
	ortb "gitlab.com/twinbid-exchange/RTB-exchange/internal/grpc/proto/types/ortb_V2_5"
)

// NativeFieldMask is compiled once from DSP rules. Only fields represented by
// this mask are materialized while parsing native.request.
type NativeFieldMask uint16

const (
	NativeNeedAssetID NativeFieldMask = 1 << iota
	NativeNeedRequired
	NativeNeedTitleLen
	NativeNeedDataType
	NativeNeedDataLen
	NativeNeedImgType
	NativeNeedImgW
	NativeNeedImgH
	NativeNeedImgWMin
	NativeNeedImgHMin
)

const NativeNeedAnyNested = NativeNeedAssetID | NativeNeedRequired | NativeNeedTitleLen |
	NativeNeedDataType | NativeNeedDataLen | NativeNeedImgType | NativeNeedImgW |
	NativeNeedImgH | NativeNeedImgWMin | NativeNeedImgHMin

func NativeMaskForField(field FieldType) NativeFieldMask {
	switch string(field) {
	case string(FieldNativeAssetID), "native.assets.id":
		return NativeNeedAssetID
	case string(FieldNativeRequired), "native.assets.required":
		return NativeNeedRequired
	case string(FieldNativeTitleLen), "native.assets.title.len":
		return NativeNeedTitleLen
	case string(FieldNativeDataType), "native.assets.data.type":
		return NativeNeedDataType
	case string(FieldNativeDataLen), "native.assets.data.len":
		return NativeNeedDataLen
	case string(FieldNativeImgType), "native.assets.img.type":
		return NativeNeedImgType
	case string(FieldNativeImgW), "native.assets.img.w":
		return NativeNeedImgW
	case string(FieldNativeImgH), "native.assets.img.h":
		return NativeNeedImgH
	case string(FieldNativeImgWMin), "native.assets.img.wmin":
		return NativeNeedImgWMin
	case string(FieldNativeImgHMin), "native.assets.img.hmin":
		return NativeNeedImgHMin
	default:
		return 0
	}
}

type nativeFilterCache struct {
	assetIDs []int
	required []int
	titleLen []int
	dataType []int
	dataLen  []int
	imgType  []int
	imgW     []int
	imgH     []int
	imgWMin  []int
	imgHMin  []int
}

// V25RequestContext is request-local and intentionally has no synchronization:
// Router evaluates DSP rules for a request sequentially. native.request is
// parsed lazily and at most once for the request context.
type bannerFormatPair struct {
	w, h       int
	hasW, hasH bool
}

type V25RequestContext struct {
	request    *ortb.BidRequest
	format     string
	nativeMask NativeFieldMask

	nativeParsed     bool
	nativeOK         bool
	nativeParseCount int
	nativeCache      nativeFilterCache

	bannerPairsLoaded bool
	bannerPairs       []bannerFormatPair
	bannerPairBound   bool
	activeBannerPair  bannerFormatPair

	base *StatelessV25BidRequestExtractor
}

func NewV25RequestContext(req *ortb.BidRequest, format string, nativeMask NativeFieldMask) *V25RequestContext {
	return &V25RequestContext{
		request:    req,
		format:     strings.ToUpper(strings.TrimSpace(format)),
		nativeMask: nativeMask & NativeNeedAnyNested,
		base:       NewStatelessV25BidRequestExtractor(),
	}
}

func (c *V25RequestContext) ExtractFieldValue(field FieldType, _ interface{}) FieldValue {
	if c == nil || c.request == nil {
		return FieldValue{}
	}

	switch string(field) {
	case string(FieldImpBanner), "banner":
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetBanner() != nil {
				return PresentValue()
			}
		}
		return FieldValue{}

	case string(FieldBannerW), "banner.w":
		values := make([]int, 0, 2)
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetBanner() != nil && imp.GetBanner().W != nil {
				values = append(values, int(imp.GetBanner().GetW()))
			}
		}
		return NewPresentIntValues(values)

	case string(FieldBannerH), "banner.h":
		values := make([]int, 0, 2)
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetBanner() != nil && imp.GetBanner().H != nil {
				values = append(values, int(imp.GetBanner().GetH()))
			}
		}
		return NewPresentIntValues(values)

	case string(FieldBannerFormatW), "banner.format.w":
		if c.bannerPairBound {
			if !c.activeBannerPair.hasW {
				return MissingValue(ValueTypeInt)
			}
			return NewPresentIntValue(c.activeBannerPair.w)
		}
		values := make([]int, 0, 4)
		for _, imp := range c.request.GetImp() {
			if imp == nil || imp.GetBanner() == nil {
				continue
			}
			for _, size := range imp.GetBanner().GetFormat() {
				if size != nil && size.W != nil {
					values = append(values, int(size.GetW()))
				}
			}
		}
		return NewPresentIntValues(values)

	case string(FieldBannerFormatH), "banner.format.h":
		if c.bannerPairBound {
			if !c.activeBannerPair.hasH {
				return MissingValue(ValueTypeInt)
			}
			return NewPresentIntValue(c.activeBannerPair.h)
		}
		values := make([]int, 0, 4)
		for _, imp := range c.request.GetImp() {
			if imp == nil || imp.GetBanner() == nil {
				continue
			}
			for _, size := range imp.GetBanner().GetFormat() {
				if size != nil && size.H != nil {
					values = append(values, int(size.GetH()))
				}
			}
		}
		return NewPresentIntValues(values)

	case string(FieldBannerMimes), "banner.mimes":
		values := make([]string, 0, 4)
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetBanner() != nil {
				values = append(values, imp.GetBanner().GetMimes()...)
			}
		}
		return NewPresentStringValues(values)

	case string(FieldImpNative), "native":
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetNative() != nil {
				return PresentValue()
			}
		}
		return FieldValue{}

	case string(FieldNativeRequest), "native.request":
		values := make([]string, 0, 1)
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetNative() != nil && imp.GetNative().Request != nil {
				values = append(values, imp.GetNative().GetRequest())
			}
		}
		return NewPresentStringValues(values)

	case string(FieldNativeVer), "native.ver":
		values := make([]string, 0, 1)
		for _, imp := range c.request.GetImp() {
			if imp != nil && imp.GetNative() != nil && imp.GetNative().Ver != nil {
				values = append(values, imp.GetNative().GetVer())
			}
		}
		return NewPresentStringValues(values)
	}

	if mask := NativeMaskForField(field); mask != 0 {
		if c.format != constants.NAT && c.format != constants.IPP {
			return MissingValue(ValueTypeInt)
		}
		if !c.ensureNativeParsed() {
			return MissingValue(ValueTypeInt)
		}
		switch mask {
		case NativeNeedAssetID:
			return NewPresentIntValues(c.nativeCache.assetIDs)
		case NativeNeedRequired:
			return NewPresentIntValues(c.nativeCache.required)
		case NativeNeedTitleLen:
			return NewPresentIntValues(c.nativeCache.titleLen)
		case NativeNeedDataType:
			return NewPresentIntValues(c.nativeCache.dataType)
		case NativeNeedDataLen:
			return NewPresentIntValues(c.nativeCache.dataLen)
		case NativeNeedImgType:
			return NewPresentIntValues(c.nativeCache.imgType)
		case NativeNeedImgW:
			return NewPresentIntValues(c.nativeCache.imgW)
		case NativeNeedImgH:
			return NewPresentIntValues(c.nativeCache.imgH)
		case NativeNeedImgWMin:
			return NewPresentIntValues(c.nativeCache.imgWMin)
		case NativeNeedImgHMin:
			return NewPresentIntValues(c.nativeCache.imgHMin)
		}
	}

	return c.base.ExtractFieldValue(field, c.request)
}

func (c *V25RequestContext) bannerFormatPairs() []bannerFormatPair {
	if c == nil || c.request == nil {
		return nil
	}
	if c.bannerPairsLoaded {
		return c.bannerPairs
	}
	c.bannerPairsLoaded = true
	for _, imp := range c.request.GetImp() {
		if imp == nil || imp.GetBanner() == nil {
			continue
		}
		for _, size := range imp.GetBanner().GetFormat() {
			if size == nil {
				continue
			}
			pair := bannerFormatPair{}
			if size.W != nil {
				pair.w, pair.hasW = int(size.GetW()), true
			}
			if size.H != nil {
				pair.h, pair.hasH = int(size.GetH()), true
			}
			c.bannerPairs = append(c.bannerPairs, pair)
		}
	}
	return c.bannerPairs
}

func (c *V25RequestContext) withBannerFormatPairs(eval func() bool) bool {
	if c == nil || eval == nil {
		return false
	}
	pairs := c.bannerFormatPairs()
	if len(pairs) == 0 {
		return false
	}
	previousBound, previousPair := c.bannerPairBound, c.activeBannerPair
	defer func() {
		c.bannerPairBound, c.activeBannerPair = previousBound, previousPair
	}()
	c.bannerPairBound = true
	for _, pair := range pairs {
		c.activeBannerPair = pair
		if eval() {
			return true
		}
	}
	return false
}

func (c *V25RequestContext) ensureNativeParsed() bool {
	if c.nativeParsed {
		return c.nativeOK
	}
	c.nativeParsed = true
	if c.nativeMask == 0 {
		return false
	}
	for _, imp := range c.request.GetImp() {
		if imp == nil || imp.GetNative() == nil || strings.TrimSpace(imp.GetNative().GetRequest()) == "" {
			continue
		}
		c.nativeParseCount++
		c.nativeOK = parseNativeFilterJSON(imp.GetNative().GetRequest(), c.nativeMask, &c.nativeCache)
		return c.nativeOK
	}
	return false
}

func parseNativeFilterJSON(raw string, mask NativeFieldMask, cache *nativeFilterCache) bool {
	if cache == nil || strings.TrimSpace(raw) == "" || mask == 0 {
		return false
	}
	iter := jsoniter.ParseString(jsoniter.ConfigFastest, raw)
	if iter == nil {
		return false
	}
	parseNativeTopObject(iter, mask, cache)
	return iter.Error == nil
}

func parseNativeTopObject(iter *jsoniter.Iterator, mask NativeFieldMask, cache *nativeFilterCache) {
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "native":
			parseNativePayloadObject(iter, mask, cache)
		case "assets":
			parseNativeAssets(iter, mask, cache)
		default:
			iter.Skip()
		}
	}
}

func parseNativePayloadObject(iter *jsoniter.Iterator, mask NativeFieldMask, cache *nativeFilterCache) {
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		if field == "assets" {
			parseNativeAssets(iter, mask, cache)
		} else {
			iter.Skip()
		}
	}
}

func parseNativeAssets(iter *jsoniter.Iterator, mask NativeFieldMask, cache *nativeFilterCache) {
	for iter.ReadArray() {
		for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
			switch field {
			case "id":
				if mask&NativeNeedAssetID != 0 {
					cache.assetIDs = append(cache.assetIDs, iter.ReadInt())
				} else {
					iter.Skip()
				}
			case "required":
				if mask&NativeNeedRequired != 0 {
					cache.required = append(cache.required, iter.ReadInt())
				} else {
					iter.Skip()
				}
			case "title":
				parseNativeTitle(iter, mask, cache)
			case "data":
				parseNativeData(iter, mask, cache)
			case "img":
				parseNativeImage(iter, mask, cache)
			default:
				iter.Skip()
			}
		}
	}
}

func parseNativeTitle(iter *jsoniter.Iterator, mask NativeFieldMask, cache *nativeFilterCache) {
	if mask&NativeNeedTitleLen == 0 {
		iter.Skip()
		return
	}
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		if field == "len" {
			cache.titleLen = append(cache.titleLen, iter.ReadInt())
		} else {
			iter.Skip()
		}
	}
}

func parseNativeData(iter *jsoniter.Iterator, mask NativeFieldMask, cache *nativeFilterCache) {
	if mask&(NativeNeedDataType|NativeNeedDataLen) == 0 {
		iter.Skip()
		return
	}
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "type":
			if mask&NativeNeedDataType != 0 {
				cache.dataType = append(cache.dataType, iter.ReadInt())
			} else {
				iter.Skip()
			}
		case "len":
			if mask&NativeNeedDataLen != 0 {
				cache.dataLen = append(cache.dataLen, iter.ReadInt())
			} else {
				iter.Skip()
			}
		default:
			iter.Skip()
		}
	}
}

func parseNativeImage(iter *jsoniter.Iterator, mask NativeFieldMask, cache *nativeFilterCache) {
	if mask&(NativeNeedImgType|NativeNeedImgW|NativeNeedImgH|NativeNeedImgWMin|NativeNeedImgHMin) == 0 {
		iter.Skip()
		return
	}
	for field := iter.ReadObject(); field != ""; field = iter.ReadObject() {
		switch field {
		case "type":
			if mask&NativeNeedImgType != 0 {
				cache.imgType = append(cache.imgType, iter.ReadInt())
			} else {
				iter.Skip()
			}
		case "w":
			if mask&NativeNeedImgW != 0 {
				cache.imgW = append(cache.imgW, iter.ReadInt())
			} else {
				iter.Skip()
			}
		case "h":
			if mask&NativeNeedImgH != 0 {
				cache.imgH = append(cache.imgH, iter.ReadInt())
			} else {
				iter.Skip()
			}
		case "wmin":
			if mask&NativeNeedImgWMin != 0 {
				cache.imgWMin = append(cache.imgWMin, iter.ReadInt())
			} else {
				iter.Skip()
			}
		case "hmin":
			if mask&NativeNeedImgHMin != 0 {
				cache.imgHMin = append(cache.imgHMin, iter.ReadInt())
			} else {
				iter.Skip()
			}
		default:
			iter.Skip()
		}
	}
}
