-- 0018_issue_watchers: per-issue watchers (Phase 5). A user "watches" an issue
-- to receive notifications about its activity. Users are auto-subscribed when
-- they create, comment on, are assigned to, or are mentioned on an issue.

CREATE TABLE issue_watchers (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    issue_id        UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (issue_id, user_id)
);

CREATE INDEX issue_watchers_user_idx ON issue_watchers (user_id);
