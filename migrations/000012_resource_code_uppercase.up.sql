-- 000011_resource_code_uppercase: enforces case-insensitive uniqueness on
-- the catalog master codes (resource_classes/families/types.code,
-- unit_definitions.code, resource_option_sets.code) so a future write that
-- bypasses domain canonicalization can never create a case-variant
-- duplicate of an existing code (e.g. "conductores" beside "CONDUCTORES").
-- Verified against production data: every persisted code is already
-- uppercase, so this migration requires no data cleanup.
-- resource_option_sets.code stays PRIMARY KEY (it is an FK target for
-- attribute_options/attribute_option_relations/resource_attributes/
-- resource_attribute_values); the upper() index is purely additive there.

BEGIN;

ALTER TABLE public.resource_classes DROP CONSTRAINT resource_classes_code_key;
CREATE UNIQUE INDEX resource_classes_code_upper_key ON public.resource_classes (UPPER(code));

ALTER TABLE public.resource_families DROP CONSTRAINT resource_families_class_id_code_key;
CREATE UNIQUE INDEX resource_families_class_id_code_upper_key ON public.resource_families (class_id, UPPER(code));

ALTER TABLE public.resource_types DROP CONSTRAINT resource_types_family_id_code_key;
CREATE UNIQUE INDEX resource_types_family_id_code_upper_key ON public.resource_types (family_id, UPPER(code));

ALTER TABLE public.unit_definitions DROP CONSTRAINT unit_definitions_code_key;
CREATE UNIQUE INDEX unit_definitions_code_upper_key ON public.unit_definitions (UPPER(code));

CREATE UNIQUE INDEX resource_option_sets_code_upper_key ON public.resource_option_sets (UPPER(code));

COMMIT;
