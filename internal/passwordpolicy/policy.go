package passwordpolicy

import "unicode/utf8"

const MinCharacters = 5
const MaxBytes = 72
const Message = "Kata sandi minimal 5 karakter, maksimal 72 byte."

// bcrypt limits bytes; count Unicode code points for the minimum instead.
func Valid(password string) bool {
	return utf8.ValidString(password) && utf8.RuneCountInString(password) >= MinCharacters && len(password) <= MaxBytes
}
