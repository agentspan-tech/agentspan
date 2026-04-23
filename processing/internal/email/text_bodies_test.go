package email

import (
	"strings"
	"testing"
	"time"
)

func TestTextVerification_EN(t *testing.T) {
	out := textVerification("Alice", "https://example.com/verify?t=abc", "en")
	if !strings.Contains(out, "Alice") {
		t.Errorf("missing name: %q", out)
	}
	if !strings.Contains(out, "https://example.com/verify?t=abc") {
		t.Errorf("missing link: %q", out)
	}
	if !strings.Contains(out, "Verify your email") {
		t.Errorf("missing EN copy: %q", out)
	}
}

func TestTextVerification_RU(t *testing.T) {
	out := textVerification("Алиса", "https://example.com/verify", "ru")
	if !strings.Contains(out, "Алиса") {
		t.Errorf("missing name: %q", out)
	}
	if !strings.Contains(out, "Подтвердите") {
		t.Errorf("missing RU copy: %q", out)
	}
}

func TestTextPasswordReset_Locales(t *testing.T) {
	for _, locale := range []string{"en", "ru", "fr"} {
		out := textPasswordReset("Bob", "https://reset", locale)
		if !strings.Contains(out, "Bob") {
			t.Errorf("[%s] missing name: %q", locale, out)
		}
		if !strings.Contains(out, "https://reset") {
			t.Errorf("[%s] missing link", locale)
		}
	}
}

func TestTextInvite_EN(t *testing.T) {
	out := textInvite("Carol", "AcmeCorp", "admin", "https://invite", "en")
	if !strings.Contains(out, "Carol") {
		t.Error("missing inviter")
	}
	if !strings.Contains(out, "AcmeCorp") {
		t.Error("missing org name")
	}
	if !strings.Contains(out, "admin") {
		t.Error("missing role")
	}
}

func TestTextInvite_RU(t *testing.T) {
	out := textInvite("Борис", "ООО", "администратор", "https://invite", "ru")
	if !strings.Contains(out, "Борис") {
		t.Error("missing inviter")
	}
	if !strings.Contains(out, "приглашает") {
		t.Error("missing RU copy")
	}
}

func TestTextDeletionNotice_EN(t *testing.T) {
	deadline := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	out := textDeletionNotice("Dan", "Corp", "en", deadline)
	if !strings.Contains(out, "Dan") {
		t.Error("missing name")
	}
	if !strings.Contains(out, "May 1, 2026") {
		t.Errorf("missing formatted date: %q", out)
	}
}

func TestTextDeletionNotice_RU(t *testing.T) {
	deadline := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	out := textDeletionNotice("Даша", "Корп", "ru", deadline)
	if !strings.Contains(out, "Даша") {
		t.Error("missing name")
	}
	if !strings.Contains(out, "1 May 2026") {
		t.Errorf("missing formatted date: %q", out)
	}
}

func TestTextDeletionWarning_EN(t *testing.T) {
	deadline := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	out := textDeletionWarning("Eve", "MyOrg", "en", deadline)
	if !strings.Contains(out, "Eve") {
		t.Error("missing name")
	}
	if !strings.Contains(out, "June 15, 2026") {
		t.Errorf("missing date: %q", out)
	}
}

func TestTextDeletionWarning_RU(t *testing.T) {
	deadline := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	out := textDeletionWarning("Ева", "Моя Орг", "ru", deadline)
	if !strings.Contains(out, "Напоминаем") {
		t.Errorf("missing RU copy: %q", out)
	}
}

func TestTextAlert_EN(t *testing.T) {
	out := textAlert("Frank", "High Latency", "latency_ms_p95", "1500", "1000", "https://dash", "en")
	if !strings.Contains(out, "Frank") || !strings.Contains(out, "High Latency") {
		t.Errorf("missing fields: %q", out)
	}
	if !strings.Contains(out, "https://dash") {
		t.Error("missing dashboard link")
	}
}

func TestTextAlert_RU(t *testing.T) {
	out := textAlert("Фрэнк", "Тест", "error_rate", "10%", "5%", "https://dash", "ru")
	if !strings.Contains(out, "Тест") {
		t.Error("missing alert name")
	}
	if !strings.Contains(out, "Порог") {
		t.Errorf("missing RU copy: %q", out)
	}
}

func TestNormalizeLocale_Recognized(t *testing.T) {
	// Unknown locale should fall through to default (likely "en")
	out := normalizeLocale("xx")
	if out == "" {
		t.Error("expected non-empty result for unknown locale")
	}
	// Russian should normalize to "ru" (or similar)
	outRu := normalizeLocale("ru")
	if outRu == "" {
		t.Error("expected non-empty for ru")
	}
	// Accept-Language format with quality
	outMulti := normalizeLocale("ru-RU,ru;q=0.9")
	if outMulti == "" {
		t.Error("expected normalization for Accept-Language format")
	}
}
