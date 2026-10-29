-- Provider notices to a mailbox about itself (sign-in alerts, 2-Step Verification, tips); old releases ignore the column.
ALTER TABLE unibox_emails ADD COLUMN IF NOT EXISTS system_notice boolean NOT NULL DEFAULT false;

-- Backfill every workspace with ingest's treatment; senders mirror internal/pkg/systemmail, re-running changes nothing.
UPDATE unibox_emails ue
SET system_notice = true,
    automated = true,
    seen = true,
    folder = CASE WHEN ue.folder = 'inbox' THEN 'archive' ELSE ue.folder END,
    updated_at = NOW()
WHERE (
        public.unibox_participants(ue.from_addr, ue.to_addr, ue.cc) ILIKE '%no-reply@accounts.google.com%'
     OR public.unibox_participants(ue.from_addr, ue.to_addr, ue.cc) ILIKE '%mail-noreply@google.com%'
     OR public.unibox_participants(ue.from_addr, ue.to_addr, ue.cc) ILIKE '%workspace-noreply@google.com%'
     OR public.unibox_participants(ue.from_addr, ue.to_addr, ue.cc) ILIKE '%account-security-noreply@accountprotection.microsoft.com%'
     OR public.unibox_participants(ue.from_addr, ue.to_addr, ue.cc) ILIKE '%msonlineservicesteam@microsoftonline.com%'
     OR public.unibox_participants(ue.from_addr, ue.to_addr, ue.cc) ILIKE '%no-reply@microsoft.com%'
  )
  AND ue.folder NOT IN ('sent', 'drafts')
  AND ue.provider_folder NOT IN ('sent', 'drafts')
  AND (
        SELECT lower(coalesce(
            substring(f.addr from '<([^<>]*)>[^<>]*$'),
            substring(f.addr from '^[[:space:]]*([^[:space:]()<>"]+@[^[:space:]()<>"]+)[[:space:]]*\('),
            substring(f.addr from '\(([^()]*)\)\s*$'),
            btrim(f.addr)
        ))
        FROM unnest(ue.from_addr) WITH ORDINALITY AS f(addr, n)
        WHERE f.addr LIKE '%@%'
        ORDER BY f.n
        LIMIT 1
      ) IN (
        'no-reply@accounts.google.com',
        'mail-noreply@google.com',
        'workspace-noreply@google.com',
        'account-security-noreply@accountprotection.microsoft.com',
        'msonlineservicesteam@microsoftonline.com',
        'no-reply@microsoft.com'
      )
  AND NOT (ue.system_notice AND ue.automated AND ue.seen AND ue.folder <> 'inbox');
