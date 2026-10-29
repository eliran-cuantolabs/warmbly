package repository

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/warmbly/warmbly/internal/infrastructure/db"
	"github.com/warmbly/warmbly/internal/models"
)

// Provider notices to a mailbox about itself land archived, read and out of every working view and counter:
//
//	WARMBLY_TEST_DB=postgres://warmbly@127.0.0.1:15432/warmbly_dev?sslmode=disable \
//	  go test ./internal/repository/ -run LiveUniboxSystemNotice -v

// liveFreshUniboxDB is a freshly migrated database of its own, so the backfill can run against it.
func liveFreshUniboxDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("WARMBLY_TEST_DB")
	if dsn == "" {
		t.Skip("WARMBLY_TEST_DB not set")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("system notice fixture requires local PostgreSQL")
	}
	admin, err := pgx.Connect(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "system_notice_" + uuid.New().String()[:8]
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(t.Context(), "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(context.Background())
		t.Fatal(err)
	}
	u.Path = "/" + name
	if err := db.RunMigrations(u.String()); err != nil {
		t.Fatal(err)
	}
	handle, err := db.New(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		handle.Pool.Close()
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+quoted); err != nil {
			t.Error(err)
		}
		_ = admin.Close(context.Background())
	})
	return handle
}

type storedPlacement struct {
	folder, provider              string
	seen, automated, systemNotice bool
	updatedAt                     time.Time
}

func readPlacement(t *testing.T, handle *db.DB, id uuid.UUID) storedPlacement {
	t.Helper()
	var p storedPlacement
	err := handle.Pool.QueryRow(context.Background(), `
		SELECT folder, provider_folder, seen, automated, system_notice, updated_at
		FROM unibox_emails WHERE id = $1`, id).
		Scan(&p.folder, &p.provider, &p.seen, &p.automated, &p.systemNotice, &p.updatedAt)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return p
}

func TestLiveUniboxSystemNoticeLandsArchivedAndOutOfEveryView(t *testing.T) {
	handle := liveFreshUniboxDB(t)
	f := newUniboxFolderFixture(t, handle.Pool)
	repo := NewUniboxRepository(handle)
	ctx := context.Background()
	now := time.Now().UTC()

	alert := f.scopedMessage(t, repo, "thread-alert", "Google <no-reply@accounts.google.com>", models.FolderInbox, now)
	ms := f.scopedMessage(t, repo, "thread-ms", "Microsoft account team <account-security-noreply@accountprotection.microsoft.com>", models.FolderInbox, now)
	spoof := f.scopedMessage(t, repo, "thread-spoof", "Google <attacker@x.com>", models.FolderInbox, now)
	f.scopedMessage(t, repo, "thread-person", "them@example.com", models.FolderInbox, now)

	for _, id := range []uuid.UUID{alert, ms} {
		p := readPlacement(t, handle, id)
		if p.folder != models.FolderArchive || p.provider != models.FolderInbox || !p.seen || !p.automated || !p.systemNotice {
			t.Fatalf("notice stored as %+v, want archived, provider inbox, seen, automated, system_notice", p)
		}
	}
	if p := readPlacement(t, handle, spoof); p.folder != models.FolderInbox || p.seen || p.systemNotice {
		t.Fatalf("display-name spoof stored as %+v, want an unread inbox message", p)
	}

	// The provider still reports the notice in its inbox; sync must not read that as a move back.
	if folder, _, moved := models.ResolveFolderSync(models.FolderArchive, models.FolderInbox, models.FolderInbox); moved || folder != models.FolderArchive {
		t.Fatalf("sync moved the notice to %q", folder)
	}

	search := func(p models.MailSearchParams) *models.MailSearchResult {
		t.Helper()
		p.PageSize = 50
		res, err := repo.Search(ctx, f.org, &p)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		return res
	}
	no, yes := false, true
	inbox, archive := models.FolderInbox, models.FolderArchive
	hidden := func(label string, res *models.MailSearchResult) {
		t.Helper()
		if listsThread(res, "thread-alert") || listsThread(res, "thread-ms") {
			t.Fatalf("%s = %v, want no system notice", label, threadIDs(res))
		}
	}
	hidden("inbox", search(models.MailSearchParams{Folder: &inbox, Automated: &no}))
	hidden("all mail", search(models.MailSearchParams{IncludeArchived: &yes}))
	hidden("unread", search(models.MailSearchParams{Unseen: &yes, Automated: &no}))
	hidden("automated", search(models.MailSearchParams{Automated: &yes}))
	hidden("awaiting reply", search(models.MailSearchParams{AwaitingReply: &yes}))
	if got := search(models.MailSearchParams{IncludeArchived: &yes}); !listsThread(got, "thread-spoof") || !listsThread(got, "thread-person") {
		t.Fatalf("all mail = %v, want the spoof and the person", threadIDs(got))
	}
	if got := search(models.MailSearchParams{Folder: &archive}); !listsThread(got, "thread-alert") || !listsThread(got, "thread-ms") {
		t.Fatalf("archive = %v, want both notices", threadIDs(got))
	}
	query := "Scope"
	if got := search(models.MailSearchParams{IncludeArchived: &yes, Subject: &query}); !listsThread(got, "thread-alert") {
		t.Fatalf("all mail search = %v, want the notice findable", threadIDs(got))
	}

	ov, err := repo.Overview(ctx, f.org)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if ov.Total != 2 || ov.Unread != 2 || ov.Automated != 0 {
		t.Fatalf("overview total=%d unread=%d automated=%d, want 2, 2, 0", ov.Total, ov.Unread, ov.Automated)
	}
	for _, fo := range ov.Folders {
		if fo.Folder == models.FolderArchive && fo.Total != 0 {
			t.Fatalf("archive folder total = %d, want 0", fo.Total)
		}
	}
	if n, err := repo.GetUnseenCount(ctx, f.org, nil); err != nil || n != 2 {
		t.Fatalf("unread badge = %d (%v), want 2", n, err)
	}

	// A later classifier verdict never turns a notice back into a person's mail.
	err = NewInboxTagRepository(handle.Pool).Save(ctx, &InboxTagResult{
		OrganizationID: f.org, EmailAccountID: f.mailbox,
		MessageID: "<" + alert.String() + "@test.local>", ThreadID: "thread-alert",
		Kind: "human_reply", KindConfidence: 0.9, KindSource: "model", Priority: "whenever",
	})
	if err != nil {
		t.Fatalf("Save verdict: %v", err)
	}
	if p := readPlacement(t, handle, alert); !p.automated {
		t.Fatal("a human verdict cleared automated on a system notice")
	}
}

func TestLiveUniboxSystemNoticeBackfillIsIdempotent(t *testing.T) {
	handle := liveFreshUniboxDB(t)
	f := newUniboxFolderFixture(t, handle.Pool)
	repo := NewUniboxRepository(handle)
	ctx := context.Background()
	now := time.Now().UTC()

	// Rows stored before the rule: written as ordinary mail, then given the sender they arrived with.
	legacy := func(thread, from, folder string) uuid.UUID {
		t.Helper()
		id := f.scopedMessage(t, repo, thread, "them@example.com", folder, now)
		if _, err := handle.Pool.Exec(ctx, `UPDATE unibox_emails SET from_addr = $2 WHERE id = $1`, id, []string{from}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	google := legacy("t-google", "Google <no-reply@accounts.google.com>", models.FolderInbox)
	gmail := legacy("t-gmail", "Gmail Team <mail-noreply@google.com>", models.FolderInbox)
	spam := legacy("t-spam", "Microsoft <no-reply@microsoft.com>", models.FolderSpam)
	lookalike := legacy("t-look", "Google <no-reply@accounts.google.com.evil.io>", models.FolderInbox)
	spoof := legacy("t-spoof", "Google <attacker@x.com>", models.FolderInbox)
	cc := legacy("t-cc", "them@example.com", models.FolderInbox)
	if _, err := handle.Pool.Exec(ctx, `UPDATE unibox_emails SET cc = ARRAY['no-reply@accounts.google.com'] WHERE id = $1`, cc); err != nil {
		t.Fatal(err)
	}

	up, err := os.ReadFile("../infrastructure/db/migrations/000331_unibox_system_notices.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	for _, id := range []uuid.UUID{google, gmail} {
		if p := readPlacement(t, handle, id); p.folder != models.FolderArchive || p.provider != models.FolderInbox || !p.seen || !p.automated || !p.systemNotice {
			t.Fatalf("backfilled notice = %+v, want archived, seen, automated, system_notice", p)
		}
	}
	if p := readPlacement(t, handle, spam); p.folder != models.FolderSpam || !p.systemNotice || !p.seen {
		t.Fatalf("notice in spam = %+v, want left in spam, marked", p)
	}
	for _, id := range []uuid.UUID{lookalike, spoof, cc} {
		if p := readPlacement(t, handle, id); p.folder != models.FolderInbox || p.seen || p.systemNotice {
			t.Fatalf("non-notice touched: %+v", p)
		}
	}

	before := readPlacement(t, handle, google)
	tag, err := handle.Pool.Exec(ctx, string(up))
	if err != nil {
		t.Fatalf("backfill rerun: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("rerun updated %d rows, want 0", tag.RowsAffected())
	}
	if after := readPlacement(t, handle, google); !after.updatedAt.Equal(before.updatedAt) {
		t.Fatal("rerun touched an already treated row")
	}
}
