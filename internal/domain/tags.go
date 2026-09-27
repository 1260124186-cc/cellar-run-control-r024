package domain

const (
	MaxFormulaTags = 16
	MaxTagLength   = 40
)

func NormalizeTag(value string) string {
	return NormalizeName(value)
}

func TagKey(value string) string {
	return NameKey(value)
}

func ValidateTags(tags []string) error {
	if len(tags) > MaxFormulaTags {
		return FieldError(CodeInvalidInput, "tags", "must contain at most 16 entries")
	}
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag == "" {
			return FieldError(CodeInvalidInput, "tags", "must not contain empty entries")
		}
		if len([]rune(tag)) > MaxTagLength {
			return FieldError(CodeInvalidInput, "tags",
				"entries must not exceed 40 characters")
		}
		key := TagKey(tag)
		if _, duplicate := seen[key]; duplicate {
			return NewError(CodeDuplicateTag,
				"the tag \""+tag+"\" appears more than once")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func NormalizeTags(tags []string) ([]string, error) {
	normalized := make([]string, 0, len(tags))
	for _, raw := range tags {
		normalized = append(normalized, NormalizeTag(raw))
	}
	if err := ValidateTags(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}
