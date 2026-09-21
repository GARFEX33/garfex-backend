-- 000013_supplier_product_mapping: current SupplierProduct mapping authority,
-- append-only confirmed-transition audit, and line-specific overrides.
BEGIN;

ALTER TABLE public.supplier_products
    ADD COLUMN mapping_revision BIGINT NOT NULL DEFAULT 0 CHECK (mapping_revision >= 0),
    ADD COLUMN mapping_identity_conflict BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE public.purchase_lines
    ADD COLUMN resolution_override TEXT NOT NULL DEFAULT 'NONE'
        CHECK (resolution_override IN ('NONE', 'NO_APLICA', 'CONFLICTO'));

UPDATE public.purchase_lines
   SET resolution_override = CASE
       WHEN link_status IN ('NO_APLICA', 'CONFLICTO') THEN link_status
       ELSE 'NONE'
   END;

CREATE TABLE public.supplier_product_mapping_audit (
    id BIGSERIAL PRIMARY KEY,
    supplier_product_id BIGINT NOT NULL,
    previous_resource_id BIGINT,
    new_resource_id BIGINT,
    previous_identity_conflict BOOLEAN NOT NULL DEFAULT FALSE,
    new_identity_conflict BOOLEAN NOT NULL DEFAULT FALSE,
    previous_revision BIGINT NOT NULL CHECK (previous_revision >= 0),
    new_revision BIGINT NOT NULL CHECK (new_revision = previous_revision + 1),
    operation TEXT NOT NULL CHECK (operation IN (
        'CONFIRM', 'CORRECT', 'EXCEPTIONAL_UNLINK',
        'REPORT_IDENTITY_CONFLICT', 'RESOLVE_IDENTITY_CONFLICT'
    )),
    actor TEXT NOT NULL CHECK (btrim(actor) <> ''),
    origin TEXT NOT NULL CHECK (origin = 'manual'),
    reason TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT supplier_product_mapping_audit_supplier_fkey
        FOREIGN KEY (supplier_product_id)
        REFERENCES public.supplier_products(id) ON DELETE RESTRICT,
    CONSTRAINT supplier_product_mapping_audit_previous_resource_fkey
        FOREIGN KEY (previous_resource_id)
        REFERENCES public.recursos(id) ON DELETE RESTRICT,
    CONSTRAINT supplier_product_mapping_audit_new_resource_fkey
        FOREIGN KEY (new_resource_id)
        REFERENCES public.recursos(id) ON DELETE RESTRICT
);

CREATE INDEX supplier_product_mapping_audit_product_idx
    ON public.supplier_product_mapping_audit (supplier_product_id, id);
CREATE INDEX supplier_products_mapping_resource_idx
    ON public.supplier_products (resource_id)
    WHERE resource_id IS NOT NULL;

ALTER TABLE public.purchase_lines DROP CONSTRAINT purchase_lines_link_status_check;
DROP INDEX IF EXISTS public.purchase_lines_link_status_idx;
ALTER TABLE public.purchase_lines DROP COLUMN link_status;

ALTER TABLE public.supplier_product_mapping_audit OWNER TO garfex_admin;
GRANT SELECT, INSERT ON public.supplier_product_mapping_audit TO garfex_app;
GRANT USAGE, SELECT ON SEQUENCE public.supplier_product_mapping_audit_id_seq TO garfex_app;
GRANT SELECT, INSERT, UPDATE ON public.supplier_products TO garfex_app;
GRANT SELECT, INSERT, UPDATE ON public.purchase_lines TO garfex_app;

COMMIT;
