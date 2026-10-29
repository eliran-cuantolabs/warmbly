// Package systemmail recognises a provider's notices to a mailbox about itself (sign-in alerts, 2-Step Verification, tips).
package systemmail

import (
	"slices"
	"strings"

	"github.com/warmbly/warmbly/internal/pkg/mailhdr"
)

// providerNoticeSenders is matched exactly on the parsed address, never a display name or suffix.
// Migration 000331 backfills from a copy of this list; TestMigrationCoversEverySender keeps them in step.
var providerNoticeSenders = map[string]struct{}{
	"no-reply@accounts.google.com":                             {}, // Google sign-in, security and 2-Step Verification alerts
	"mail-noreply@google.com":                                  {}, // Gmail welcome and tips
	"workspace-noreply@google.com":                             {}, // Google Workspace service and legal notices
	"account-security-noreply@accountprotection.microsoft.com": {}, // Microsoft account security and sign-in alerts
	"msonlineservicesteam@microsoftonline.com":                 {}, // Microsoft 365 service notices
	"no-reply@microsoft.com":                                   {}, // Microsoft account notices
}

// Senders returns the recognised addresses, sorted.
func Senders() []string {
	out := make([]string, 0, len(providerNoticeSenders))
	for addr := range providerNoticeSenders {
		out = append(out, addr)
	}
	slices.Sort(out)
	return out
}

// IsProviderNotice reports whether the first From entry that parses to an address is a provider notice sender.
func IsProviderNotice(from []string) bool {
	for _, raw := range from {
		addr := strings.ToLower(strings.TrimSpace(mailhdr.Bare(raw)))
		if !strings.Contains(addr, "@") {
			continue
		}
		_, ok := providerNoticeSenders[addr]
		return ok
	}
	return false
}
