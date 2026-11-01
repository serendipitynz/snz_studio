// Package lang names the processing language: the language of the bundled
// presets and of the instructions the app sends to the LLM. It is separate from
// the display language, which only the frontend knows.
package lang

// Language is a processing language. The zero value is not one; Parse and
// config.Config hand out only the two below.
type Language string

const (
	Japanese Language = "ja"
	English  Language = "en"
)

// Parse reports whether value names a processing language.
func Parse(value string) (Language, bool) {
	switch Language(value) {
	case Japanese, English:
		return Language(value), true
	}
	return "", false
}
