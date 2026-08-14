package filter

func compareIntScalar(fieldInt int, cond ConditionType, values [2]int) bool {
	switch cond {
	case ConditionEqual:
		return fieldInt == values[0]
	case ConditionNotEqual:
		return fieldInt != values[0]
	case ConditionGreaterThan:
		return fieldInt > values[0]
	case ConditionGreaterEqual:
		return fieldInt >= values[0]
	case ConditionLessThan:
		return fieldInt < values[0]
	case ConditionLessEqual:
		return fieldInt <= values[0]
	case ConditionBetween:
		return fieldInt >= values[0] && fieldInt <= values[1]
	case ConditionNotBetween:
		return fieldInt < values[0] || fieldInt > values[1]
	default:
		return false
	}
}

type IntCondition struct {
	values [2]int
	cond   ConditionType
}

func (ic IntCondition) Type() ValueType { return ValueTypeInt }
func (ic IntCondition) Compare(fieldValue FieldValue) bool {
	if fieldValue.Type != ValueTypeInt && !(ic.cond == ConditionExists && fieldValue.PresenceAware) {
		return false
	}
	if ic.cond == ConditionExists {
		if fieldValue.PresenceAware {
			return fieldValue.Present
		}
		// Preserve legacy numeric exists behavior.
		return true
	}
	if fieldValue.PresenceAware && !fieldValue.Present {
		return false
	}
	if len(fieldValue.Ints) == 0 {
		return compareIntScalar(fieldValue.Int, ic.cond, ic.values)
	}
	if ic.cond == ConditionNotEqual || ic.cond == ConditionNotBetween {
		for _, value := range fieldValue.Ints {
			if !compareIntScalar(value, ic.cond, ic.values) {
				return false
			}
		}
		return true
	}
	for _, value := range fieldValue.Ints {
		if compareIntScalar(value, ic.cond, ic.values) {
			return true
		}
	}
	return false
}

type StringCondition struct {
	value string
	cond  ConditionType
}

func (sc StringCondition) Type() ValueType { return ValueTypeString }
func (sc StringCondition) Compare(fieldValue FieldValue) bool {
	if fieldValue.Type != ValueTypeString && !(sc.cond == ConditionExists && fieldValue.PresenceAware) {
		return false
	}
	if sc.cond == ConditionExists {
		if fieldValue.PresenceAware {
			return fieldValue.Present
		}
		// Preserve legacy string exists behavior.
		return fieldValue.String != ""
	}
	if fieldValue.PresenceAware && !fieldValue.Present {
		return false
	}
	compare := func(value string) bool {
		switch sc.cond {
		case ConditionEqual:
			return value == sc.value
		case ConditionNotEqual:
			return value != sc.value
		default:
			return false
		}
	}
	if len(fieldValue.Strings) == 0 {
		return compare(fieldValue.String)
	}
	if sc.cond == ConditionNotEqual {
		for _, value := range fieldValue.Strings {
			if !compare(value) {
				return false
			}
		}
		return true
	}
	for _, value := range fieldValue.Strings {
		if compare(value) {
			return true
		}
	}
	return false
}

type FloatCondition struct {
	values [2]float64
	cond   ConditionType
}

func (fc FloatCondition) Type() ValueType { return ValueTypeFloat }
func (fc FloatCondition) Compare(fieldValue FieldValue) bool {
	if fieldValue.Type != ValueTypeFloat && !(fc.cond == ConditionExists && fieldValue.PresenceAware) {
		return false
	}
	if fc.cond == ConditionExists {
		if fieldValue.PresenceAware {
			return fieldValue.Present
		}
		// Preserve legacy numeric exists behavior.
		return true
	}
	if fieldValue.PresenceAware && !fieldValue.Present {
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
	default:
		return false
	}
}
