package validate

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var (
	emailPattern    = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
	phonePattern    = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	languagePattern = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)
)

func Email(value string) (string, string) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) > 254 || !emailPattern.MatchString(value) {
		return "", "Enter a valid email address."
	}
	return value, ""
}

func Password(value string) string {
	if len(value) < 10 {
		return "Use at least 10 characters."
	}
	if len(value) > 128 {
		return "Use at most 128 characters."
	}
	return ""
}

func DisplayName(value string) (string, string) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) < 1 || len([]rune(value)) > 80 {
		return "", "Enter a name between 1 and 80 characters."
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", "Name contains unsupported characters."
		}
	}
	return value, ""
}

func Phone(value string) (string, string) {
	value = strings.TrimSpace(value)
	if !phonePattern.MatchString(value) {
		return "", "Enter a phone number in international format, such as +9198XXXXXXXX."
	}
	return value, ""
}

func Language(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "en", ""
	}
	if !languagePattern.MatchString(value) {
		return "", "Use a language code such as en or hi."
	}
	return value, ""
}

func ID(value string) (uuid.UUID, string) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, "Invalid identifier."
	}
	return id, ""
}

func OrgType(value string) string {
	switch value {
	case "municipality", "utility", "business", "ngo", "institution", "service_provider", "other":
		return ""
	default:
		return "Choose a valid organization type."
	}
}

func AccountKind(value string) string {
	switch value {
	case "resident", "provider", "organization":
		return ""
	default:
		return "Choose resident, provider, or organization."
	}
}
