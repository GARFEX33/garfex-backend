BEGIN;

ALTER TABLE public.resource_type_presentation_fields DROP COLUMN IF EXISTS id;
ALTER TABLE public.resource_unit_policies DROP COLUMN IF EXISTS id;
ALTER TABLE public.attribute_options DROP COLUMN IF EXISTS id;
ALTER TABLE public.resource_option_sets DROP COLUMN IF EXISTS id;

COMMIT;
