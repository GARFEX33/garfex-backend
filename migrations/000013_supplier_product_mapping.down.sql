-- Rollback restores only the compatible legacy derived projection. The
-- append-only mapping audit and revision/conflict authority are intentionally
-- discarded because legacy schema has no faithful storage for them.
BEGIN;

DROP INDEX IF EXISTS public.supplier_products_mapping_resource_idx;

ALTER TABLE public.purchase_lines
    ADD COLUMN link_status TEXT NOT NULL DEFAULT 'PENDIENTE'
        CHECK (link_status IN ('PENDIENTE', 'VINCULADO', 'NO_APLICA', 'CONFLICTO'));

UPDATE public.purchase_lines pl
   SET link_status = CASE
       WHEN pl.resolution_override IN ('NO_APLICA', 'CONFLICTO') THEN pl.resolution_override
       WHEN sp.mapping_identity_conflict THEN 'CONFLICTO'
       WHEN sp.resource_id IS NOT NULL AND r.active = FALSE THEN 'PENDIENTE'
       WHEN sp.resource_id IS NOT NULL THEN 'VINCULADO'
       ELSE 'PENDIENTE'
   END
  FROM public.supplier_products sp
  LEFT JOIN public.recursos r ON r.id = sp.resource_id
 WHERE sp.id = pl.supplier_product_id;

DROP INDEX IF EXISTS public.purchase_lines_link_status_idx;
CREATE INDEX purchase_lines_link_status_idx
    ON public.purchase_lines (link_status)
    WHERE link_status = 'PENDIENTE';

ALTER TABLE public.purchase_lines DROP CONSTRAINT IF EXISTS purchase_lines_resolution_override_check;
ALTER TABLE public.purchase_lines DROP COLUMN IF EXISTS resolution_override;

REVOKE ALL ON SEQUENCE public.supplier_product_mapping_audit_id_seq FROM garfex_app;
DROP TABLE IF EXISTS public.supplier_product_mapping_audit;

ALTER TABLE public.supplier_products DROP COLUMN IF EXISTS mapping_identity_conflict;
ALTER TABLE public.supplier_products DROP COLUMN IF EXISTS mapping_revision;

COMMIT;
