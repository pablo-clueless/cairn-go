-- 0025_space_members: per-space membership. A user only sees/accesses a space if
-- they're a member of it (org owners/admins implicitly access all spaces). This
-- narrows the previous org-wide visibility to space-scoped visibility.
--
-- Backfill: grant every current org member access to every existing space, so no
-- one loses access to spaces they can see today. The restriction applies to new
-- spaces and to membership changes going forward.

CREATE TABLE space_members (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    space_id        UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (space_id, user_id)
);

CREATE INDEX space_members_user_idx ON space_members (organization_id, user_id);

INSERT INTO space_members (organization_id, space_id, user_id)
SELECT s.organization_id, s.id, m.user_id
FROM spaces s
JOIN memberships m ON m.organization_id = s.organization_id
ON CONFLICT (space_id, user_id) DO NOTHING;
