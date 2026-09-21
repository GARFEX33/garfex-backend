-- 000014_purchase_line_resolution_audit: optimistic revision and append-only
-- manual-decision audit for purchase-line resolution overrides.
BEGIN;

ALTER TABLE public.purchase_lines
    ADD COLUMN resolution_revision BIGINT NOT NULL DEFAULT 0
        CHECK (resolution_revision >= 0);

CREATE TABLE public.purchase_line_resolution_audit (
    id BIGSERIAL PRIMARY KEY,
    purchase_line_id BIGINT NOT NULL,
    previous_override TEXT NOT NULL
        CHECK (previous_override IN ('NONE', 'NO_APLICA', 'CONFLICTO')),
    new_override TEXT NOT NULL
        CHECK (new_override IN ('NONE', 'NO_APLICA', 'CONFLICTO')),
    previous_revision BIGINT NOT NULL CHECK (previous_revision >= 0),
    new_revision BIGINT NOT NULL CHECK (new_revision = previous_revision + 1),
    actor TEXT NOT NULL CHECK (btrim(actor) <> ''),
    origin TEXT NOT NULL CHECK (origin = 'manual'),
    reason TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT purchase_line_resolution_audit_line_fkey
        FOREIGN KEY (purchase_line_id)
        REFERENCES public.purchase_lines(id) ON DELETE RESTRICT
);

CREATE INDEX purchase_line_resolution_audit_line_idx
    ON public.purchase_line_resolution_audit (purchase_line_id, id);

ALTER TABLE public.purchase_line_resolution_audit OWNER TO garfex_admin;
GRANT SELECT, INSERT ON public.purchase_line_resolution_audit TO garfex_app;
GRANT USAGE, SELECT ON SEQUENCE public.purchase_line_resolution_audit_id_seq TO garfex_app;
GRANT SELECT, UPDATE ON public.purchase_lines TO garfex_app;

COMMIT;
