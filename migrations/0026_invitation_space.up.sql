-- 0026_invitation_space: let an invitation target a specific space. NULL keeps
-- the existing org-only behavior; when set, accepting the invite also adds the
-- user to that space (in addition to the org). The pending-per-email-per-org
-- unique index is unchanged (one outstanding invite per email per org).

ALTER TABLE invitations
    ADD COLUMN space_id UUID REFERENCES spaces(id) ON DELETE CASCADE;
