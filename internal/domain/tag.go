package domain

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxFormulaTags = 25
	MaxTagRunes    = 40
)

// NormalizeTag lower-cases a tag and applies the same whitespace folding as
// formula names so that " Summer  Ale " and "summer ale" are the same tag.
func NormalizeTag(value string) string {
	return strings.ToLower(NormalizeName(value))
}

// ValidateTag checks a normalized tag. Tags use letters, digits, underscores,
// hyphens, and single internal spaces only, and must begin and end with a
// letter or digit.
func ValidateTag(value, field string) error {
	if value == "" {
		return FieldError(CodeInvalidInput, field, "is required")
	}
	if utf8.RuneCountInString(value) > MaxTagRunes {
		return FieldError(CodeInvalidInput, field,
			"must not exceed 40 characters")
	}
	first, _ := utf8.DecodeRuneInString(value)
	last, _ := utf8.DecodeLastRuneInString(value)
	if !isTagTokenRune(first) || !isTagTokenRune(last) {
		return FieldError(CodeInvalidInput, field,
			"must begin and end with a letter or digit")
	}
	for _, r := range value {
		if isTagTokenRune(r) || r == ' ' || r == '-' || r == '_' {
			continue
		}
		return FieldError(CodeInvalidInput, field,
			"may contain only letters, digits, spaces, hyphens, and underscores")
	}
	return nil
}

func isTagTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// NormalizeTags normalizes, validates, de-duplicates, and sorts a tag list.
// Repeated tags after normalization are reported with CodeDuplicateTag so the
// caller never silently drops a maintenance request.
func NormalizeTags(values []string) ([]string, error) {
	if len(values) > MaxFormulaTags {
		return nil, FieldError(CodeInvalidInput, "tags",
			"must contain at most 25 entries")
	}
	seen := make(map[string]struct{}, len(values))
	tags := make([]string, 0, len(values))
	for _, raw := range values {
		tag := NormalizeTag(raw)
		if err := ValidateTag(tag, "tags"); err != nil {
			return nil, err
		}
		if _, exists := seen[tag]; exists {
			return nil, NewError(CodeDuplicateTag,
				"tag \""+tag+"\" is listed more than once")
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags, nil
}

func HasTag(tags []string, tag string) bool {
	for _, existing := range tags {
		if existing == tag {
			return true
		}
	}
	return false
}

// NormalizeStoredTags folds tags read from a snapshot into canonical form.
// Snapshots are only ever written through validated maintenance paths, so this
// is defensive normalization for older files rather than new validation.
func NormalizeStoredTags(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	tags := make([]string, 0, len(values))
	for _, raw := range values {
		tag := NormalizeTag(raw)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
