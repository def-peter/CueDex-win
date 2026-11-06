// Package i18n picks the interface language. The setting follows Windows until
// the user picks one, so a fresh install speaks the language of the system it
// was installed on.
package i18n

import "syscall"

// Language is a language the interface is written in.
type Language string

const (
	// System means follow Windows.
	System  Language = ""
	Chinese Language = "zh"
	English Language = "en"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// Resolve turns the stored setting into a language the panel can draw.
func Resolve(setting string) Language {
	return ResolveWith(setting, SystemLanguage())
}

// ResolveWith is Resolve with the system language supplied, so a test does not
// depend on the machine it runs on.
func ResolveWith(setting string, system Language) Language {
	switch Language(setting) {
	case Chinese:
		return Chinese
	case English:
		return English
	}
	if system == Chinese {
		return Chinese
	}
	return English
}

// SystemLanguage is the language Windows is using. The primary language id is
// the low ten bits of the LANGID; 0x04 is Chinese.
func SystemLanguage() Language {
	langid, _, _ := procGetUserDefaultUILanguage.Call()
	if uint16(langid)&0x3ff == 0x04 {
		return Chinese
	}
	return English
}
