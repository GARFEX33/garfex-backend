BEGIN;

DROP INDEX IF EXISTS public.purchase_line_resolution_audit_line_idx;
DROP TABLE IF EXISTS public.purchase_line_resolution_audit;
ALTER TABLE public.purchase_lines DROP COLUMN IF EXISTS resolution_revision;

COMMIT;
