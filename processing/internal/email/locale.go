package email

import "fmt"

// normalizeLocale returns the supported locale for email templates.
// Falls back to "en" for anything other than "ru".
func normalizeLocale(locale string) string {
	if locale == "ru" {
		return "ru"
	}
	return "en"
}

// templateName returns the locale-specific template filename for a given base.
// Example: templateName("verification", "ru") -> "verification_ru.html".
func templateName(base, locale string) string {
	return fmt.Sprintf("%s_%s.html", base, normalizeLocale(locale))
}

// subjects maps (template key, locale) -> Subject header.
// Subjects that interpolate a value (org name, alert name) are handled inline
// in the SMTPMailer methods via fmt.Sprintf on the looked-up template.
var subjects = map[string]map[string]string{
	"verification": {
		"en": "Verify your AgentOrbit email",
		"ru": "Подтвердите email в AgentOrbit",
	},
	"password_reset": {
		"en": "Reset your AgentOrbit password",
		"ru": "Сброс пароля в AgentOrbit",
	},
	"invite": {
		"en": "You've been invited to join %s on AgentOrbit",
		"ru": "Вас пригласили в %s на AgentOrbit",
	},
	"deletion_notice": {
		"en": "Your organization %q is scheduled for deletion",
		"ru": "Организация %q запланирована к удалению",
	},
	"deletion_warning": {
		"en": "Reminder: %q will be deleted soon",
		"ru": "Напоминание: %q скоро будет удалена",
	},
	"alert": {
		"en": "[AgentOrbit] Alert: %s",
		"ru": "[AgentOrbit] Оповещение: %s",
	},
}

func subject(key, locale string) string {
	if m, ok := subjects[key]; ok {
		if v, ok := m[normalizeLocale(locale)]; ok {
			return v
		}
	}
	return "AgentOrbit"
}
