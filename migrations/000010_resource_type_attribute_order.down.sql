-- Reversing this migration removes visual preferences only.
BEGIN;

REVOKE SELECT, INSERT, UPDATE, DELETE ON
    public.resource_type_attribute_order_items,
    public.resource_type_attribute_orders
FROM garfex_app;

DROP TABLE public.resource_type_attribute_order_items;
DROP TABLE public.resource_type_attribute_orders;

COMMIT;
