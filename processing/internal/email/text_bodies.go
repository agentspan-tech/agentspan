package email

import (
	"fmt"
	"time"
)

// Plain-text fallbacks shown when the recipient's client does not render HTML.
// One function per template × locale; kept simple (no template engine) because
// the surface is small and readability trumps cleverness.

func textVerification(name, link, locale string) string {
	switch normalizeLocale(locale) {
	case "ru":
		return fmt.Sprintf("Здравствуйте, %s!\n\nПодтвердите ваш email:\n%s\n\nЕсли вы не регистрировались в AgentOrbit, проигнорируйте это письмо.\n", name, link)
	default:
		return fmt.Sprintf("Hi %s,\n\nVerify your email address:\n%s\n\nIf you did not sign up for AgentOrbit, you can ignore this email.\n", name, link)
	}
}

func textPasswordReset(name, link, locale string) string {
	switch normalizeLocale(locale) {
	case "ru":
		return fmt.Sprintf("Здравствуйте, %s!\n\nСброс пароля AgentOrbit:\n%s\n\nСсылка действует 1 час. Если вы не запрашивали сброс пароля, проигнорируйте это письмо.\n", name, link)
	default:
		return fmt.Sprintf("Hi %s,\n\nReset your AgentOrbit password:\n%s\n\nThis link expires in 1 hour. If you did not request a password reset, you can ignore this email.\n", name, link)
	}
}

func textInvite(inviterName, orgName, role, link, locale string) string {
	switch normalizeLocale(locale) {
	case "ru":
		return fmt.Sprintf("Здравствуйте!\n\n%s приглашает вас присоединиться к %s в роли %s.\n\nПринять приглашение:\n%s\n\nСсылка действует 7 дней.\n", inviterName, orgName, role, link)
	default:
		return fmt.Sprintf("Hi,\n\n%s has invited you to join %s as %s.\n\nAccept your invite:\n%s\n\nThis link expires in 7 days.\n", inviterName, orgName, role, link)
	}
}

func textDeletionNotice(name, orgName, locale string, scheduledAt time.Time) string {
	switch normalizeLocale(locale) {
	case "ru":
		return fmt.Sprintf("Здравствуйте, %s!\n\nВаша организация %q запланирована к безвозвратному удалению %s.\n\nУ вас 14 дней, чтобы отменить это действие в настройках организации.\n", name, orgName, scheduledAt.Format("2 January 2006"))
	default:
		return fmt.Sprintf("Hi %s,\n\nYour organization %q has been scheduled for deletion on %s.\n\nYou have 14 days to cancel this action from the organization settings.\n", name, orgName, scheduledAt.Format("January 2, 2006"))
	}
}

func textDeletionWarning(name, orgName, locale string, deletionAt time.Time) string {
	switch normalizeLocale(locale) {
	case "ru":
		return fmt.Sprintf("Здравствуйте, %s!\n\nНапоминаем, что ваша организация %q будет безвозвратно удалена %s.\n\nЧтобы отменить удаление, зайдите в настройки организации до этой даты.\n", name, orgName, deletionAt.Format("2 January 2006"))
	default:
		return fmt.Sprintf("Hi %s,\n\nThis is a reminder that your organization %q will be permanently deleted on %s.\n\nTo cancel, visit your organization settings before the deletion date.\n", name, orgName, deletionAt.Format("January 2, 2006"))
	}
}

func textAlert(name, alertName, alertType, currentValue, threshold, dashboardLink, locale string) string {
	switch normalizeLocale(locale) {
	case "ru":
		return fmt.Sprintf("Здравствуйте, %s!\n\nСработало оповещение %q.\n\nТип: %s\nТекущее значение: %s\nПорог: %s\n\nПерейти в дашборд: %s\n", name, alertName, alertType, currentValue, threshold, dashboardLink)
	default:
		return fmt.Sprintf("Hi %s,\n\nAlert \"%s\" has triggered.\n\nType: %s\nCurrent value: %s\nThreshold: %s\n\nView dashboard: %s\n", name, alertName, alertType, currentValue, threshold, dashboardLink)
	}
}
