package filter

type IntCondition struct {
	values [2]int // использование array вместо slice
	cond   ConditionType
	// Убрать hasValues - оно не используется
}

func (ic IntCondition) Type() ValueType { return ValueTypeInt }
func (ic IntCondition) Compare(fieldValue FieldValue) bool {
	if fieldValue.Type != ValueTypeInt {
		return false
	}

	fieldInt := fieldValue.Int
	switch ic.cond {
	case ConditionEqual:
		return fieldInt == ic.values[0]
	case ConditionNotEqual:
		return fieldInt != ic.values[0]
	case ConditionGreaterThan:
		return fieldInt > ic.values[0]
	case ConditionGreaterEqual:
		return fieldInt >= ic.values[0]
	case ConditionLessThan:
		return fieldInt < ic.values[0]
	case ConditionLessEqual:
		return fieldInt <= ic.values[0]
	case ConditionBetween:
		return fieldInt >= ic.values[0] && fieldInt <= ic.values[1]
	case ConditionNotBetween:
		return fieldInt < ic.values[0] || fieldInt > ic.values[1]
	case ConditionExists:
		return true
	default:
		return false
	}
}

type StringCondition struct {
	value string // одно значение вместо slice
	cond  ConditionType
}

func (sc StringCondition) Type() ValueType { return ValueTypeString }
func (sc StringCondition) Compare(fieldValue FieldValue) bool {
	if fieldValue.Type != ValueTypeString {
		return false
	}

	switch sc.cond {
	case ConditionEqual:
		return fieldValue.String == sc.value
	case ConditionNotEqual:
		return fieldValue.String != sc.value
	case ConditionExists:
		return fieldValue.String != ""
	default:
		return false
	}
}

type FloatCondition struct {
	values [2]float64 // использование array вместо slice
	cond   ConditionType
}

func (fc FloatCondition) Type() ValueType { return ValueTypeFloat }
func (fc FloatCondition) Compare(fieldValue FieldValue) bool {
	if fieldValue.Type != ValueTypeFloat {
		return false
	}

	fieldFloat := fieldValue.Float
	switch fc.cond {
	case ConditionEqual:
		return fieldFloat == fc.values[0]
	case ConditionNotEqual:
		return fieldFloat != fc.values[0]
	case ConditionGreaterThan:
		return fieldFloat > fc.values[0]
	case ConditionGreaterEqual:
		return fieldFloat >= fc.values[0]
	case ConditionLessThan:
		return fieldFloat < fc.values[0]
	case ConditionLessEqual:
		return fieldFloat <= fc.values[0]
	case ConditionBetween:
		return fieldFloat >= fc.values[0] && fieldFloat <= fc.values[1]
	case ConditionNotBetween:
		return fieldFloat < fc.values[0] || fieldFloat > fc.values[1]
	case ConditionExists:
		return true
	default:
		return false
	}
}
