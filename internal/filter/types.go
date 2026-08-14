package filter

import (
	"encoding/json"
)

type FieldType string

const (
	FieldDeviceCountry FieldType = "device.geo.country"
	FieldAppID         FieldType = "app.id"
	FieldSiteID        FieldType = "site.id"
	FieldDeviceIP      FieldType = "device.ip"
	FieldBidNurl       FieldType = "bid.nurl"
	FieldBidBurl       FieldType = "bid.burl"

	FieldBidPrice    FieldType = "bid.price"
	FieldBidID       FieldType = "bid.id"
	FieldBidAdID     FieldType = "bid.adid"
	FieldBidImpID    FieldType = "bid.impid"
	FieldSeatBidSeat FieldType = "seatbid.seat"
	FieldBidArray    FieldType = "bid.array"

	FieldSitePage       FieldType = "site.page"
	FieldSiteDomain     FieldType = "site.domain"
	FieldDeviceUA       FieldType = "device.ua"
	FieldDeviceLanguage FieldType = "device.language"
	FieldUserID         FieldType = "user.id"
	FieldAuctionType    FieldType = "at"
	FieldTMax           FieldType = "tmax"

	FieldBidRequestID   FieldType = "bidrequest.id"
	FieldBidRequestAt   FieldType = "bidrequest.at"
	FieldBidRequestTMax FieldType = "bidrequest.tmax"
	FieldBidRequestCur  FieldType = "bidrequest.cur"
	FieldBidRequestBCat FieldType = "bidrequest.bcat"

	FieldImpID FieldType = "imp.id"

	// OpenRTB impression-format fields used by DSP request filters.
	FieldImpBanner      FieldType = "imp.banner"
	FieldBannerW        FieldType = "imp.banner.w"
	FieldBannerH        FieldType = "imp.banner.h"
	FieldBannerFormatW  FieldType = "imp.banner.format.w"
	FieldBannerFormatH  FieldType = "imp.banner.format.h"
	FieldBannerMimes    FieldType = "imp.banner.mimes"
	FieldImpNative      FieldType = "imp.native"
	FieldNativeRequest  FieldType = "imp.native.request"
	FieldNativeVer      FieldType = "imp.native.ver"
	FieldNativeAssetID  FieldType = "imp.native.assets.id"
	FieldNativeRequired FieldType = "imp.native.assets.required"
	FieldNativeTitleLen FieldType = "imp.native.assets.title.len"
	FieldNativeDataType FieldType = "imp.native.assets.data.type"
	FieldNativeDataLen  FieldType = "imp.native.assets.data.len"
	FieldNativeImgType  FieldType = "imp.native.assets.img.type"
	FieldNativeImgW     FieldType = "imp.native.assets.img.w"
	FieldNativeImgH     FieldType = "imp.native.assets.img.h"
	FieldNativeImgWMin  FieldType = "imp.native.assets.img.wmin"
	FieldNativeImgHMin  FieldType = "imp.native.assets.img.hmin"

	// Новые поля для Site
	FieldSiteName FieldType = "site.name"
	FieldSiteRef  FieldType = "site.ref"
	FieldSiteCat  FieldType = "site.cat"

	// Новые поля для BidResponse
	FieldBidResponseID    FieldType = "bidresponse.id"
	FieldBidResponseBidID FieldType = "bidresponse.bidid"

	// Новые поля для Bid
	FieldBidAdm FieldType = "bid.adm"
)

type ValueType string

const (
	ValueTypeInt    ValueType = "int"
	ValueTypeFloat  ValueType = "float"
	ValueTypeString ValueType = "string"
)

type ConditionType string

const (
	ConditionEqual        ConditionType = "equal"
	ConditionNotEqual     ConditionType = "not_equal"
	ConditionGreaterThan  ConditionType = "greater_than"
	ConditionGreaterEqual ConditionType = "greater_equal"
	ConditionLessThan     ConditionType = "less_than"
	ConditionLessEqual    ConditionType = "less_equal"
	ConditionBetween      ConditionType = "between"
	ConditionNotBetween   ConditionType = "not_between"
	ConditionExists       ConditionType = "exists"
)

type FieldValue struct {
	Type          ValueType
	Int           int
	Float         float64
	String        string
	Ints          []int
	Strings       []string
	Present       bool
	PresenceAware bool
}

// Legacy scalar constructors intentionally preserve the original filter
// semantics for existing POP/common fields.
func NewIntValue(value int) FieldValue {
	return FieldValue{Type: ValueTypeInt, Int: value}
}

func NewFloatValue(value float64) FieldValue {
	return FieldValue{Type: ValueTypeFloat, Float: value}
}

func NewStringValue(value string) FieldValue {
	return FieldValue{Type: ValueTypeString, String: value}
}

// Presence-aware constructors are used only for the new OpenRTB format fields.
// They distinguish a missing numeric field from an explicitly present zero.
func NewPresentIntValue(value int) FieldValue {
	return FieldValue{Type: ValueTypeInt, Int: value, Present: true, PresenceAware: true}
}

func NewPresentIntValues(values []int) FieldValue {
	if len(values) == 0 {
		return MissingValue(ValueTypeInt)
	}
	return FieldValue{Type: ValueTypeInt, Int: values[0], Ints: values, Present: true, PresenceAware: true}
}

func NewPresentStringValue(value string) FieldValue {
	return FieldValue{Type: ValueTypeString, String: value, Present: true, PresenceAware: true}
}

func NewPresentStringValues(values []string) FieldValue {
	if len(values) == 0 {
		return MissingValue(ValueTypeString)
	}
	return FieldValue{Type: ValueTypeString, String: values[0], Strings: values, Present: true, PresenceAware: true}
}

func PresentValue() FieldValue {
	return FieldValue{Present: true, PresenceAware: true}
}

func MissingValue(valueType ValueType) FieldValue {
	return FieldValue{Type: valueType, PresenceAware: true}
}

type ConditionValue interface {
	Type() ValueType
	Compare(value FieldValue) bool
}

type FilterRule struct {
	ID        string
	Field     FieldType
	Condition ConditionType
	Value     ConditionValue
}

type SimpleRuleConfig struct {
	Version string                 `json:"version"`
	DSPs    map[string]DSPSettings `json:"dsps"`
	SPPs    map[string]SPPSettings `json:"spps"`
}

type RuleNode struct {
	// Для простых правил
	Field     FieldType       `json:"field,omitempty"`
	Condition ConditionType   `json:"condition,omitempty"`
	ValueType ValueType       `json:"value_type,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`

	// Для группировки (рекурсивно)
	AND []RuleNode `json:"and,omitempty"`
	OR  []RuleNode `json:"or,omitempty"`
}

type DSPSettings struct {
	Rules []RuleNode `json:"rules"`
}

type SPPSettings struct {
	Rules []RuleNode `json:"rules"`
}

// Добавляем структуры для поддержки версий
type VersionedDSPSettings struct {
	V25 []RuleNode `json:"v25,omitempty"`
}

type VersionedSPPSettings struct {
	V25 []RuleNode `json:"v25,omitempty"`
}

type VersionedRuleConfig struct {
	Version string                          `json:"version"`
	DSPs    map[string]VersionedDSPSettings `json:"dsps,omitempty"`
	SPPs    map[string]VersionedSPPSettings `json:"spps,omitempty"`
}

type SimpleRule struct {
	Field     FieldType       `json:"field"`
	Condition ConditionType   `json:"condition"`
	ValueType ValueType       `json:"value_type"`
	Value     json.RawMessage `json:"value"`
}

type FilterResult struct {
	Allowed bool `json:"allowed"`
}

// BidRequestExtractor интерфейс для stateless извлечения значений
type BidRequestExtractor interface {
	ExtractFieldValue(field FieldType, req interface{}) FieldValue
}

// BidResponseExtractor интерфейс для stateless извлечения значений
type BidResponseExtractor interface {
	ExtractFieldValue(field FieldType, resp interface{}) FieldValue
}
