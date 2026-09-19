-- Separate visual order for all effective bindings of one Tipo.
-- Positions are zero-based. No presentation or resource identity is changed.
-- Membership and contiguous ordering are validated by the Core transaction.
BEGIN;

CREATE TABLE public.resource_type_attribute_orders (
    type_id BIGINT PRIMARY KEY REFERENCES public.resource_types(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE public.resource_type_attribute_order_items (
    target_type_id BIGINT NOT NULL REFERENCES public.resource_type_attribute_orders(type_id) ON DELETE CASCADE,
    resource_attribute_id BIGINT NOT NULL REFERENCES public.resource_attributes(id) ON DELETE CASCADE,
    position INTEGER NOT NULL CHECK (position >= 0),
    PRIMARY KEY (target_type_id, resource_attribute_id),
    UNIQUE (target_type_id, position)
);

-- Binding deletion must find its preferences across every inheriting Tipo.
CREATE INDEX resource_type_attribute_order_items_binding_idx
    ON public.resource_type_attribute_order_items (resource_attribute_id);

ALTER TABLE public.resource_type_attribute_orders OWNER TO garfex_admin;
ALTER TABLE public.resource_type_attribute_order_items OWNER TO garfex_admin;

GRANT SELECT, INSERT, UPDATE, DELETE ON
    public.resource_type_attribute_orders,
    public.resource_type_attribute_order_items
TO garfex_app;

COMMIT;
