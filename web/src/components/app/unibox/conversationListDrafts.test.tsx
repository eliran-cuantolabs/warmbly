// The Drafts folder lists threads with a pending inbox-agent draft above the mailbox's own drafts.

import React from "react";
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { UniboxSearchParams } from "@/lib/api/models/app/unibox/UniboxSearch";
import { ConversationList } from "./ConversationList";

const search = vi.hoisted(() => ({ calls: [] as { params: UniboxSearchParams; enabled: boolean }[] }));

function row(id: string, thread: string, subject: string) {
  return {
    id,
    thread_id: thread,
    email_id: "m1",
    from_addr: ["Lead <lead@example.com>"],
    to_addr: ["me@example.com"],
    subject,
    snippet: subject,
    internal_date: new Date().toISOString(),
    has_unread: false,
    message_count: 1,
    labels: [],
  };
}

vi.mock("@/lib/api/hooks/app/unibox/useUniboxSearch", () => ({
  default: (params: UniboxSearchParams, _scopeKey: string, enabled = true) => {
    search.calls.push({ params, enabled });
    const emails = !enabled
      ? []
      : params.agentDrafts
        ? [row("a1", "t-agent", "Agent reply to a lead"), row("a2", "t-both", "Agent reply on a drafted thread")]
        : [row("d1", "t-mine", "My own draft"), row("d2", "t-both", "Provider copy of a drafted thread")];
    return {
      emails,
      isPending: false,
      isFetching: false,
      isPlaceholderData: false,
      isError: false,
      hasNextPage: false,
      isFetchingNextPage: false,
      isFetchNextPageError: false,
      fetchNextPage: () => Promise.resolve(),
      refetch: () => Promise.resolve(),
    };
  },
}));
vi.mock("./ConversationItem", () => ({
  ConversationItem: ({ email }: { email: { subject: string } }) => <div data-testid="row">{email.subject}</div>,
}));
vi.mock("./AnimatedRow", () => ({
  AnimatedRow: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
vi.mock("./SelectionBar", () => ({ SelectionBar: () => null }));
vi.mock("./UniboxFilterPopover", () => ({
  countUserFilters: () => 0,
  UniboxFilterButton: () => null,
  UniboxFilterChips: () => null,
}));
vi.mock("@/components/app/unibox/compose/ComposeButton", () => ({ default: () => null }));
vi.mock("@/hooks/useConversationActions", () => ({ useConversationActions: () => ({}) }));
vi.mock("@/hooks/useShortcutActions", () => ({ useShortcutActions: () => {} }));
vi.mock("@/hooks/usePermission", () => ({ useAccessRestricted: () => false }));
vi.mock("@/hooks/useBrowseDebouncedValue", () => ({ default: (v: unknown) => v }));
vi.mock("@/hooks/useScrollMemory", () => ({ useScrollMemory: () => {} }));
vi.mock("framer-motion", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  AnimatePresence: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

afterEach(() => {
  cleanup();
  search.calls = [];
});

function mount(params: UniboxSearchParams, scopeKey: string) {
  return render(
    <ConversationList
      scopeKey={scopeKey}
      scopeLabel="Drafts"
      params={params}
      baseParams={params}
      setParams={() => {}}
      search=""
      setSearch={() => {}}
    />,
  );
}

it("lists pending agent drafts first under their own header, then the mailbox drafts", () => {
  mount({ sortBy: "newest", folder: "drafts" }, "folder:drafts");

  expect(screen.getByText("Agent drafts")).toBeTruthy();
  expect(screen.getAllByTestId("row").map((r) => r.textContent)).toEqual([
    "Agent reply to a lead",
    "Agent reply on a drafted thread",
    "My own draft",
  ]);
  const agent = search.calls.find((c) => c.params.agentDrafts);
  expect(agent?.enabled).toBe(true);
  expect(agent?.params.folder).toBeUndefined();
});

it("leaves every other folder alone", () => {
  mount({ sortBy: "newest", folder: "inbox", automated: false }, "folder:inbox");

  expect(screen.queryByText("Agent drafts")).toBeNull();
  expect(search.calls.filter((c) => c.params.agentDrafts).every((c) => !c.enabled)).toBe(true);
  expect(screen.getAllByTestId("row")).toHaveLength(2);
});
