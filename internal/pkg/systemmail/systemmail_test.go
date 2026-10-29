package systemmail

import (
	"os"
	"strings"
	"testing"
)

func TestIsProviderNoticeMatchesTheParsedAddressOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		from []string
		want bool
	}{
		{"google security alert", []string{"Google <no-reply@accounts.google.com>"}, true},
		{"gmail team", []string{"Gmail Team <mail-noreply@google.com>"}, true},
		{"workspace team", []string{`"The Google Workspace Team" <workspace-noreply@google.com>`}, true},
		{"bare and uppercase", []string{"NO-REPLY@ACCOUNTS.GOOGLE.COM"}, true},
		{"legacy parenthesised form", []string{"Google (no-reply@accounts.google.com)"}, true},
		{"microsoft account security", []string{"Microsoft account team <account-security-noreply@accountprotection.microsoft.com>"}, true},
		{"microsoft 365", []string{"Microsoft Online Services Team <msonlineservicesteam@microsoftonline.com>"}, true},
		{"microsoft no-reply", []string{"Microsoft <no-reply@microsoft.com>"}, true},
		{"empty first entry skipped", []string{"", "Google <no-reply@accounts.google.com>"}, true},
		{"lookalike domain", []string{"Google <no-reply@accounts.google.com.evil.io>"}, false},
		{"lookalike prefix", []string{"Google <xno-reply@accounts.google.com>"}, false},
		{"display name spoof", []string{"Google <attacker@x.com>"}, false},
		{"address quoted in display name", []string{`"no-reply@accounts.google.com" <attacker@x.com>`}, false},
		{"other google sender", []string{"Google Calendar <calendar-notification@google.com>"}, false},
		{"only first sender counts", []string{"them@example.com", "Google <no-reply@accounts.google.com>"}, false},
		{"no sender", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsProviderNotice(tc.from); got != tc.want {
				t.Fatalf("IsProviderNotice(%q) = %v, want %v", tc.from, got, tc.want)
			}
		})
	}
}

func TestMigrationCoversEverySender(t *testing.T) {
	raw, err := os.ReadFile("../../infrastructure/db/migrations/000331_unibox_system_notices.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	for _, addr := range Senders() {
		if !strings.Contains(sql, "'%"+addr+"%'") || !strings.Contains(sql, "'"+addr+"'") {
			t.Errorf("migration 000331 does not backfill %s", addr)
		}
	}
	if got := strings.Count(sql, "ILIKE '%"); got != len(Senders()) {
		t.Errorf("migration prefilters %d senders, the list has %d", got, len(Senders()))
	}
}
