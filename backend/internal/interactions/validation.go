package interactions

import (
	"net/mail"
	"regexp"
	"unicode/utf8"
)

const (
	toggleBodyLimit  = 2 * 1024
	commentBodyLimit = 16 * 1024
	maxNameLength    = 100
	maxEmailLength   = 254
	maxCommentLength = 5000
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

func validLength(value string, maxLength int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxLength
}

func validEmail(value string) bool {
	if value == "" || len(value) > maxEmailLength {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}
