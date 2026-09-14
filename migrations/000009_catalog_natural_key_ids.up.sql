-- 000009_catalog_natural_key_ids: adds a real BIGSERIAL `id` column to the
-- four catalog kinds that were only ever addressed by their natural key
-- (resource_option_sets/CONJUNTO_OPCIONES, attribute_options/OPCION,
-- resource_unit_policies/POLITICA_UNIDAD, resource_type_presentation_fields/
-- PRESENTACION). Those four previously exposed hashtextextended(natural key)
-- as their public "id", a signed 64-bit hash that can be negative — but the
-- public write routes (PUT/deactivate/reactivate) only ever accept a strict
-- positive decimal id, so a negative-hashed row could never be addressed for
-- a write. Purely additive: the existing composite PRIMARY KEY on each table
-- is untouched (nothing that FOREIGN KEY-references it needs to change), id
-- is UNIQUE NOT NULL and auto-backfills every existing row sequentially.
-- garfex_app's existing table-level GRANTs from 000002/000003 already cover
-- this new column on these already-granted tables; no additional GRANT is
-- required.

BEGIN;

ALTER TABLE public.resource_option_sets
    ADD COLUMN id BIGSERIAL UNIQUE NOT NULL;

ALTER TABLE public.attribute_options
    ADD COLUMN id BIGSERIAL UNIQUE NOT NULL;

ALTER TABLE public.resource_unit_policies
    ADD COLUMN id BIGSERIAL UNIQUE NOT NULL;

ALTER TABLE public.resource_type_presentation_fields
    ADD COLUMN id BIGSERIAL UNIQUE NOT NULL;

COMMIT;
