-- 000011_purchases: purchase documents, their line items, and the reusable
-- supplier-product identity that bridges them to a Resource Master entry.
-- Runtime identities can create and read; historical rows are never updated
-- except for the link-status/relation columns purchases module owns.
BEGIN;

CREATE TABLE public.supplier_products (
    id BIGSERIAL PRIMARY KEY,
    supplier_id BIGINT NOT NULL,
    supplier_sku TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    resource_id BIGINT,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT supplier_products_sku_nonblank CHECK (btrim(supplier_sku) <> ''),
    CONSTRAINT supplier_products_supplier_id_fkey
        FOREIGN KEY (supplier_id) REFERENCES public.suppliers(id) ON DELETE RESTRICT,
    CONSTRAINT supplier_products_resource_id_fkey
        FOREIGN KEY (resource_id) REFERENCES public.recursos(id) ON DELETE RESTRICT,
    UNIQUE (supplier_id, supplier_sku)
);

CREATE INDEX supplier_products_resource_idx
    ON public.supplier_products (resource_id)
    WHERE resource_id IS NOT NULL;

CREATE TABLE public.purchases (
    id BIGSERIAL PRIMARY KEY,
    supplier_id BIGINT NOT NULL,
    branch_id BIGINT,
    cfdi_uuid TEXT NOT NULL,
    series TEXT NOT NULL DEFAULT '',
    folio TEXT NOT NULL DEFAULT '',
    issued_at TIMESTAMPTZ NOT NULL,
    currency TEXT NOT NULL,
    exchange_rate NUMERIC,
    subtotal NUMERIC NOT NULL,
    discount NUMERIC NOT NULL DEFAULT 0,
    tax_transferred NUMERIC NOT NULL DEFAULT 0,
    tax_withheld NUMERIC NOT NULL DEFAULT 0,
    total NUMERIC NOT NULL,
    issuer_tax_id TEXT NOT NULL,
    issuer_name TEXT NOT NULL DEFAULT '',
    xml_content BYTEA NOT NULL,
    xml_hash TEXT NOT NULL,
    xml_filename TEXT NOT NULL DEFAULT '',
    imported_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT purchases_cfdi_uuid_nonblank CHECK (btrim(cfdi_uuid) <> ''),
    CONSTRAINT purchases_xml_hash_length CHECK (char_length(xml_hash) = 64),
    CONSTRAINT purchases_supplier_id_fkey
        FOREIGN KEY (supplier_id) REFERENCES public.suppliers(id) ON DELETE RESTRICT,
    CONSTRAINT purchases_supplier_branch_fkey
        FOREIGN KEY (supplier_id, branch_id)
        REFERENCES public.supplier_branches (supplier_id, id) ON DELETE RESTRICT,
    UNIQUE (cfdi_uuid)
);

-- The purchase date drives every price-history and evolution query, so it is
-- indexed on its own in addition to the per-supplier composite below.
CREATE INDEX purchases_issued_at_idx ON public.purchases (issued_at);
CREATE INDEX purchases_supplier_idx ON public.purchases (supplier_id, issued_at);
CREATE INDEX purchases_xml_hash_idx ON public.purchases (xml_hash);

CREATE TABLE public.purchase_lines (
    id BIGSERIAL PRIMARY KEY,
    purchase_id BIGINT NOT NULL,
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    description TEXT NOT NULL,
    supplier_sku TEXT NOT NULL DEFAULT '',
    sat_product_code TEXT NOT NULL DEFAULT '',
    quantity NUMERIC NOT NULL,
    unit_code TEXT NOT NULL DEFAULT '',
    unit TEXT NOT NULL DEFAULT '',
    unit_price NUMERIC NOT NULL,
    amount NUMERIC NOT NULL,
    discount NUMERIC NOT NULL DEFAULT 0,
    tax_transferred NUMERIC NOT NULL DEFAULT 0,
    tax_withheld NUMERIC NOT NULL DEFAULT 0,
    tax_object TEXT NOT NULL DEFAULT '',
    supplier_product_id BIGINT,
    link_status TEXT NOT NULL DEFAULT 'PENDIENTE'
        CHECK (link_status IN ('PENDIENTE', 'VINCULADO', 'NO_APLICA', 'CONFLICTO')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT purchase_lines_purchase_id_fkey
        FOREIGN KEY (purchase_id) REFERENCES public.purchases(id) ON DELETE CASCADE,
    CONSTRAINT purchase_lines_supplier_product_id_fkey
        FOREIGN KEY (supplier_product_id) REFERENCES public.supplier_products(id) ON DELETE RESTRICT,
    UNIQUE (purchase_id, line_number)
);

CREATE INDEX purchase_lines_supplier_product_idx
    ON public.purchase_lines (supplier_product_id)
    WHERE supplier_product_id IS NOT NULL;
CREATE INDEX purchase_lines_link_status_idx
    ON public.purchase_lines (link_status)
    WHERE link_status = 'PENDIENTE';

CREATE TRIGGER supplier_products_set_updated_at
    BEFORE UPDATE ON public.supplier_products
    FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();
CREATE TRIGGER purchases_set_updated_at
    BEFORE UPDATE ON public.purchases
    FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();
CREATE TRIGGER purchase_lines_set_updated_at
    BEFORE UPDATE ON public.purchase_lines
    FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();

ALTER TABLE public.supplier_products OWNER TO garfex_admin;
ALTER TABLE public.purchases OWNER TO garfex_admin;
ALTER TABLE public.purchase_lines OWNER TO garfex_admin;

GRANT SELECT, INSERT, UPDATE ON
    public.supplier_products, public.purchases, public.purchase_lines TO garfex_app;
REVOKE DELETE ON
    public.supplier_products, public.purchases, public.purchase_lines FROM garfex_app;
GRANT USAGE, SELECT ON SEQUENCE
    public.supplier_products_id_seq, public.purchases_id_seq, public.purchase_lines_id_seq TO garfex_app;

COMMIT;
