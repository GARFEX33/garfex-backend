BEGIN;

DROP INDEX public.resource_option_sets_code_upper_key;

DROP INDEX public.unit_definitions_code_upper_key;
ALTER TABLE public.unit_definitions ADD CONSTRAINT unit_definitions_code_key UNIQUE (code);

DROP INDEX public.resource_types_family_id_code_upper_key;
ALTER TABLE public.resource_types ADD CONSTRAINT resource_types_family_id_code_key UNIQUE (family_id, code);

DROP INDEX public.resource_families_class_id_code_upper_key;
ALTER TABLE public.resource_families ADD CONSTRAINT resource_families_class_id_code_key UNIQUE (class_id, code);

DROP INDEX public.resource_classes_code_upper_key;
ALTER TABLE public.resource_classes ADD CONSTRAINT resource_classes_code_key UNIQUE (code);

COMMIT;
