-- Archived rows stay archived: which a person filed is not recorded; Archive and search still list them.
ALTER TABLE unibox_emails DROP COLUMN IF EXISTS system_notice;
