# Extracción de conocimiento del Resource Master anterior para GARFEX v2

**Corte auditado:** commit `bf2b777` de `main`, árbol de trabajo observado el 22 de agosto de 2026.

**Base factual:** `garfex-resource-master-current-state.md`, auditoría READ-ONLY de 1819 líneas, complementada con código, pruebas, migraciones 000002–000008 y OpenSpec histórico.
**Objetivo:** separar comportamiento legado comprobable, intención documentada y decisiones todavía abiertas. Este documento no prescribe almacenamiento, tecnología ni arquitectura futura.

## Metodología

1. Se leyó completa la auditoría factual base antes de ampliar áreas.
2. Se usó CodeGraph para recorrer dominio, aplicación, PostgreSQL, `resourcecore`, bridge y Supplier Master.
3. Se contrastaron migraciones, pruebas y OpenSpec cuando la pregunta exigía intención o contrato, priorizando código y pruebas ejecutables ante contradicciones.
4. No se ejecutaron migraciones, escrituras de base, pruebas destructivas ni `go build`. Engram no se usa como evidencia.

## Leyenda y límites

- **CONFIRMADO:** código, prueba, migración o dato observado lo demuestra.
- **INFERIDO:** consecuencia razonable de hechos confirmados, sin contrato explícito.
- **NO EXISTÍA:** se buscó deliberadamente la capacidad y estaba ausente.
- **NO DETERMINABLE:** el repositorio no permite conocer la intención, necesidad o requisito preguntado.
- **YA CONFIRMADO:** la auditoría base ya cubre completamente el punto; se resume y referencia sin repetirla.

Las referencias de líneas son estables para el corte auditado. “No encontrado” fuera de este checkout no prueba inexistencia en sistemas externos. Las decisiones RM-DOM-01..04 no aparecen como evidencia legacy porque pertenecen a v2.

# A. Canonicalización de valores

## A1. CONTROLLED_TEXT

**Clasificación principal: CONFIRMADO.** En la entrada de dominio, `canonicalValue` rechaza texto cuyo `strings.TrimSpace` queda vacío y almacena `strings.Join(strings.Fields(strings.TrimSpace(text)), " ")`: elimina whitespace exterior y colapsa cualquier secuencia interna reconocida por `Fields` a un espacio ASCII. Conserva mayúsculas/minúsculas y caracteres con acento; no aplica normalización Unicode ni elimina diacríticos (`internal/domain/resource_canonical.go:43-48`).

Hay que distinguir tres representaciones:

| Etapa | Ejemplo de entrada `"  Árbol   Azul  "` | Resultado |
|---|---|---|
| valor almacenado en `Resource.Attributes` y PostgreSQL | entrada a `NewResource` | `"Árbol Azul"` |
| valor mostrado por `Describe` | llama `value.canonical(...)` | `"ÁRBOL AZUL"` |
| fragmento de identidad, si `IdentityParticipates=true` | `identityComponent(value.canonical(...))` | `11:ÁRBOL AZUL` (11 bytes UTF-8) |

Por tanto, el valor almacenado y el mostrado **no** son necesariamente iguales. La identidad aplica después `canonical()`, que vuelve uppercase y normaliza whitespace; sí puede participar porque la participación pertenece al binding, no al tipo. Dos almacenados `"Azul"` y `"azul"` producen el mismo fragmento canónico `4:AZUL`. No hay fixture semilla ni dato vivo `CONTROLLED_TEXT`; el test de codec sólo prueba preservación de `"approved"` (`internal/postgres/resource_repository_codec_test.go:20`). La longitud es en bytes UTF-8; `TestIdentityComponentPreventsDelimiterCollision` prueba `á -> 2:á` (`resource_canonical_writes_test.go:17-23`).

**Evidencia:** `internal/domain/resource_canonical.go:43-48,68-82,92-98`; `resource_validation.go:77-89,95-97`; `resource_presentation.go:29-37`; test citado.

## A2. DECIMAL

**Clasificación principal: CONFIRMADO.** El dominio conserva `decimal.Decimal` y el valor canónico usa `shopspring/decimal.String()` sin `float64` (`resource_canonical.go:35-38,76-78`). `1`, `1.0`, `1.00` y `01.000` parsean al mismo valor y se proyectan como `"1"`; no se preserva escala. `CanonicalDecimalString` fuerza además cualquier cero a `"0"` (`resourcecore/values.go:11-20`).

No existe límite de precisión ni redondeo de negocio en Go; la biblioteca mantiene coeficiente/exponente arbitrarios. PostgreSQL usa `NUMERIC` sin precisión/escala declarada, por lo que tampoco impone redondeo de columna (`migrations/000002_resource_master.up.sql`, `resource_attribute_values.decimal_value`). La identidad es determinista porque incorpora el string canónico con longitud. La frontera pública recibe decimal como texto, lo parsea y rechaza sintaxis inválida; `TestCanonicalNumbers` prueba `1.200 -> 1.2` (`resourcecore/values_test.go:5-14`).

**Evidencia:** `internal/domain/resource_canonical.go:68-82`; `resourcecore/values.go:11-37`; `resourcecore/values_test.go`; OpenSpec base `resource-master-core/spec.md:44-53` y diseño de estabilización §Canonical numeric strings.

## A3. INTEGER

**Clasificación principal: CONFIRMADO.** La representación interna es `int64`; la identidad usa `fmt.Sprintf("%d", value)`, por lo que cualquier entrada textual aceptada que termine en el entero 1 produce `"1"` (`resource_canonical.go:74-76`). La frontera pública canonicaliza con `strconv.ParseInt(..., 10, 64)` y vuelve a emitir base 10: `01`, `1` y `+1` convergen en 1.

Los límites son exactamente `math.MinInt64..math.MaxInt64`; overflow en la frontera es `INVALID_ARGUMENT`. Dentro del dominio no hay un rango de negocio adicional. Los DTO constructores reciben directamente `int64`, eliminando ambigüedad textual (`resourcecore/values.go:9,22`).

**Evidencia:** `internal/domain/resource_types.go:121-129,144-146`; `resource_canonical.go:74-76`; `resourcecore/values.go:9,22`; OpenSpec de Create, diseño `mapValueToDomain`/`FieldInt`.

## A4. BOOLEAN

**Clasificación principal: CONFIRMADO.** La identidad usa `fmt.Sprintf("%t", bool)`: los únicos strings son `"true"` y `"false"`, en minúsculas; no `1/0` (`resource_canonical.go:78-80`). PostgreSQL persiste `BOOLEAN` y la frontera pública usa el miembro booleano del union, no texto libre.

**Evidencia:** `internal/domain/resource_canonical.go:68-82`; `resource_types.go:128,156-158`; `resourcecore/types.go:38-45`.

## A5. QUANTITY

**Clasificación principal: CONFIRMADO.** `Quantity` contiene magnitud `decimal.Decimal` y `UnitCode string`. La magnitud debe ser estrictamente positiva; el código se canonicaliza con trim, colapso y uppercase. La identidad concatena `decimal.String() + " " + canonical(UnitCode)`, por ejemplo `1 M`; `1 M` y `1.0 M` son equivalentes, pero `1 M` y `1 CM` son identidades distintas si el atributo participa. No hay conversión entre unidades ni uso del argumento `dimension` dentro de `canonicalQuantity`; dimensión sólo valida compatibilidad de unidad en la cadena activa.

No existe atributo Quantity identitario en `SeedResourceCatalog`, ni entre las ocho definiciones o 21 valores vivos observados; sí hay fixtures técnicos de cardinalidad con `2.5 M`, no evidencia de uso de producto (`resource_repository_cardinality_integration_test.go:32-46`).

**Evidencia:** `resource_types.go:116-119`; `resource_canonical.go:26-30,72-87`; `resource_validation.go:129-213`; auditoría base §§5.2–5.4, 16.2 y RM-INV-017.

# B. Applicability y reglas

## B1. Antecedentes permitidos

**Clasificación principal: CONFIRMADO.** El modelo sólo expresa `AttributeCondition{AttributeCode, Equals string}`. La evaluación busca un valor ya presente por código y compara **únicamente** `value.OptionCode == canonical(rule.When.Equals)`. En la práctica, el antecedente útil es un `CONTROLLED_OPTION`; no existen operadores de desigualdad, rango, mayor/menor, membresía múltiple, AND, OR, NOT, existencia ni ausencia.

El descriptor público obliga `ApplicabilityRule.Equals.Kind == TEXT`, pero eso sólo transporta el string comparado; no amplía el lenguaje.

**Evidencia:** `internal/domain/resource_types.go:91-102`; `resource_validation.go:327-336`; `internal/postgres/resource_repository_attributes.go:16`; `resourcecore/writer.go` y pruebas de shape de reglas.

## B2. Dependencias entre características

**Clasificación principal: CONFIRMADO.** Una regla puede nombrar cualquier código; `ResourceCatalog.Validate` no comprueba que el antecedente sea aplicable al mismo scope, su tipo ni ciclos. No existe grafo, DFS ni detector de ciclos. Por ello cadenas y ciclos pueden almacenarse si pasan referencias SQL básicas, pero la evaluación no transforma valores ni propaga resultados: siempre consulta el mapa de valores originales presentados/cargados.

Un ciclo `color→insulation→voltage→color` no causa iteración infinita porque cada `effective` sólo hace un loop finito sobre sus reglas. Tampoco obtiene semántica de punto fijo; es configuración sin efecto recursivo.

**Evidencia:** `resource_catalog_validate.go:70-80`; `resource_validation.go:27-62,327-336`; migración 000003, FK `when_definition_id`; ausencia deliberadamente buscada de validación de ciclos.

## B3. Orden de evaluación entre atributos

**Clasificación principal: CONFIRMADO.** Hay una sola pasada sobre códigos de atributo ordenados alfabéticamente, pero cada regla ve el mapa completo `byCode` construido antes; por eso el orden de atributos no altera el match de valores ya suministrados. No hay punto fijo ni segunda evaluación. Dentro de un binding gana la **primera** regla coincidente, en orden del slice; el loader lo llena por `display_order`.

Si A depende de B y B de C, ambos leen B/C originales y calculan su propio modo una vez. El resultado efectivo de B no se convierte en input de A. Las pruebas sólo cubren match y fallback (`TestResourceAttributeEffectiveAppliesConditionalRule`, `...FallsBackToRequiredWhenNoRuleMatches`), no cadenas/ciclos.

**Evidencia:** `resource_validation.go:52-82,327-336`; `catalog_loader.go:290-319`; `resource_catalog_query_test.go:286-315`.

## B4. Regla que referencia atributo ausente

**Clasificación principal: CONFIRMADO.** Si el antecedente no está en `byCode` —por ser opcional ausente, forbidden sin valor, N/A retirado durante rehidratación o no aplicable al scope— la regla no coincide. No se produce error por la referencia durante evaluación. Para un binding `CONDITIONAL`, no-match retorna implícitamente `REQUIRED`; por eso luego puede fallar por ausencia del atributo condicionado.

Un antecedente persistido `NOT_APPLICABLE` no posee `OptionCode` y tampoco coincide. Una definición inexistente está limitada por FK al escribir SQL, pero el validador de snapshot no comprueba explícitamente todos los antecedentes: si se construye sólo en memoria, también resulta no-match.

**Evidencia:** `resource_validation.go:103-120,327-336`; `resource_repository_attributes.go:16,34-52`; migración 000003 FK de reglas.

## B5. Reglas inactivas

**Clasificación principal: CONFIRMADO.** El comportamiento no es uniforme. El `effective` de dominio **no mira `rule.Active`**: una regla inactiva sigue pudiendo ganar durante `NewResource` y `Effective`. Inmediatamente antes, los writes normales ejecutan `validateActiveChain`, que rechaza el binding entero si contiene cualquier regla inactiva; no la ignora. La lectura SQL histórica sí filtra `rr.active`, por lo que una regla inactiva no participa en la reconstrucción efectiva y aplica el fallback CONDITIONAL→REQUIRED.

`HydrateResource(snapshot)` simple tampoco reevalúa reglas ni actividad; la ruta histórica preserva lo almacenado. Esto es una inconsistencia legacy, no una semántica única que deba idealizarse.

**Evidencia:** `resource_validation.go:184-188,327-336`; `resource_repository_attributes.go:16`; `resource_types.go:225-232`; auditoría base §§7.3, 19 RM-INV-010.

# C. Family inheritance y Type override

## C1. Precedencia real

**Clasificación principal: CONFIRMADO.** `resourceAttributes` recorre el slice y asigna por código en un map; el último matching gana y sustituye **todo** el `ResourceAttribute` (modo, identidad, OptionSet, reglas, Active y definición embebida). El loader ordena `family` (`type_id NULL`, `COALESCE=0`) antes del binding de tipo; por eso, con ese loader, Type reemplaza Family completamente.

La consulta histórica implementa explícitamente la misma preferencia con `NOT EXISTS override` y ranking `scope_rank`. No hay merge de campos heredados. El resultado de dominio depende del orden del slice y no existe test unitario que consagre “Type siempre último” para catálogos construidos por otro origen; por eso la precedencia de producto está confirmada para el loader PostgreSQL, mientras el mecanismo por orden es frágil.

**Evidencia:** `resource_validation.go:271-282`; `catalog_loader.go:322-355`; `resource_repository_attributes.go:9-16`; integración de cardinalidad/búsqueda citada en auditoría §11.2.

## C2. Binding específico inactivo

**Clasificación principal: CONFIRMADO.** El binding Type inactivo sigue cargándose y, por estar después, tapa al Family activo. En write, `validateActiveChain` rechaza `attribute.Active=false`; no vuelve al Family. En lectura SQL histórica, el `NOT EXISTS override` tampoco filtra `override.active`, de modo que bloquea herencia; la consulta no filtra el binding elegido por `ra.active`. No existe fallback automático al Family.

**Evidencia:** `catalog_loader.go:322-355`; `resource_validation.go:170-180,274-282`; `resource_repository_attributes.go:16`.

## C3. Desactivación

**Clasificación principal: YA CONFIRMADO.** La regla general y sus excepciones están completas en la auditoría base §§7.3, 20 y “Comportamiento no protegido”: nuevas escrituras/reactivaciones exigen cadena activa; lecturas históricas por `HydrateResource` toleran catálogo inactivo. Binding/definición/OptionSet/opción/regla/relación inactivos bloquean writes según `validateActiveChain`; Family/Type inactivos bloquean nueva creación y reactivación; la lectura simple preserva el histórico.

La desactivación de un binding Type no reactiva herencia Family (C2). `OptionsFor`, `FamiliesFor` y `TypesFor` sí filtran inactivos para consultas editoriales. Presentation no filtra `Active` al describir, otra inconsistencia concreta.

**Evidencia:** auditoría base §§7.3, 20; `resource_catalog_query.go:43-55,87-118,165-179`; `resource_presentation.go:49-57`.

# D. Options y OptionSets

## D1. Estabilidad de códigos

**Clasificación principal: YA CONFIRMADO.** Todos los campos `code` de CLASE, FAMILIA, TIPO, CARACTERISTICA, CONJUNTO_OPCIONES, OPCION y UNIDAD están declarados `ImmutableOnceReferenced`; no hay `code` propio equivalente en políticas/presentación. El guard compara código en update y consulta referencias antes de permitir cambio. “Referenced” depende del handler: hijos catalogográficos y/o recursos; por ejemplo unidad mira políticas, `natural_unit_id` y `quantity_unit_id`, opción mira relaciones y valores.

El código no es `ImmutableAlways`: un registro sin referencias puede renombrarse. La matriz completa de campos y dependencias está en auditoría §§31.1–31.2.

**Evidencia:** auditoría base §§31.1–31.2; `catalog_kind.go`; `catalogo.Service.Update`; `catalog_admin_kinds.go:1007-1014`.

## D2. Desactivar una Option usada

**Clasificación principal: CONFIRMADO.** El valor histórico sigue decodificándose porque la FK y el row permanecen. `OptionsFor` deja de ofrecer ROJO. Crear o reemplazar un Resource que incluya ROJO falla por opción inactiva; como update es full replace, “sin tocar ese campo” no existe a nivel Core: el caller debe reenviar ROJO y falla. Reactivar el Resource también reconstruye contra catálogo activo y falla mientras ROJO siga inactivo.

La lectura histórica no revalida actividad de la opción. La reactivación de la Option puede restaurar admisión si el catálogo completo vuelve a ser válido.

**Evidencia:** `resource_catalog_query.go:43-55`; `resource_validation.go:189-197`; `resource_types.go:225-232`; `recursos.Service.Update/ReactivateRevision`; auditoría §§7.3, 20.

## D3. Relaciones direccionales

**Clasificación principal: CONFIRMADO.** Los datos guardan una sola relación dirigida `diameter_inch -> diameter_mm`. La validación de un par usa exactamente from/to almacenados, pero acepta el par cuando ambos valores coinciden. La consulta editorial `ValidOptions` recorre la relación en ambos sentidos explícitamente, así que seleccionar 13 mm restringe pulgadas a `1/2"` sin almacenar la inversa.

**Evidencia:** `resource_catalog.go:124-133`; `resource_validation.go:285-324`; `resource_catalog_query.go:121-162`; tests `TestValidOptionsNarrowsDiameterMm...` y `...Inch...`.

## D4. Cardinalidad de relaciones

**Clasificación principal: CONFIRMADO.** La restricción única sólo impide repetir la misma tupla completa; una opción puede tener cero, una o varias relaciones. `validateRelations` agrupa todas las relaciones A→B y acepta si **alguna** pareja coincide. Por tanto A→B1 y A→B2 es representable y funciona como conjunto permitido; no hay restricción 1:1.

Los datos vivos usan nueve pares uno-a-uno, lo cual no constituye límite contractual.

**Evidencia:** migración 000002, UNIQUE de `attribute_option_relations`; `resource_validation.go:290-324`; auditoría §§5.3, 29.10.

## D5. Uso de numeric_value/unit_id en opciones

**Clasificación principal: CONFIRMADO.** Las columnas existen y deben ser ambas nulas o ambas no nulas, pero `loadOptions` no las selecciona; `AttributeOption` no tiene campos equivalentes; bridge, identidad, `ValidOptions`, presentación y validación de Resources no las usan. No se observaron casos reales que consuman esa equivalencia.

Son metadata SQL huérfana en el comportamiento Core auditado, no una conversión escondida.

**Evidencia:** migración 000002 y auditoría §29.9; `catalog_loader.go:358-376`; `resource_types.go:104-114`; búsqueda deliberada de `numeric_value` sin consumidores Go.

# E. Unidades

## E1. Dimension

**Clasificación principal: CONFIRMADO.** `Dimension` es string libre no vacío, no referencia un catálogo. La comparación de compatibilidad es exacta sobre strings cargados; no se canonicaliza en esa comparación, por lo que es case-sensitive. El update de UNIDAD o CARACTERISTICA permite cambiar dimensión si pasa guards/catálogo; los Resources históricos se leen sin revalidarla, pero un update/reactivación puede dejar de ser válido.

No hay contrato de vocabulario (`LENGTH`, `PIECE`, etc.) más allá de datos/fixtures.

**Evidencia:** `resource_types.go:73-89`; `resource_catalog_validate.go` validación no vacía; `resource_validation.go` validación Quantity; migración 000002.

## E2. NaturalUnit

**Clasificación principal: CONFIRMADO.** Pertenece al Resource y se valida contra la política de su Family, pero está excluida expresamente de IdentityV1. Dos intentos iguales salvo NaturalUnit derivan la misma identidad y la UNIQUE `(class_id, identity_key)` impide dos filas; la unidad es una propiedad mutable del mismo recurso, no parte de su clave de duplicado.

El “por qué” de producto más profundo es **NO DETERMINABLE**; lo documentado sólo establece que es management unit y no identidad.

**Evidencia:** `resource_validation.go:23-26,83-90`; `resource_catalog_validate.go:56-57`; `TestCanalizacionesNaturalUnitDoesNotParticipateInIdentity`; auditoría RM-INV-020/021.

## E3. Cambio de NaturalUnit

**Clasificación principal: CONFIRMADO.** Update legacy y CAS aceptan una nueva NaturalUnit, conservan `Resource.ID` y reemplazan `natural_unit_id`. No existe protección por consumidores externos ni FK entrante a Resource en este repositorio; sólo se valida política/unidad activa. La identidad no cambia sólo por unidad, salvo el defecto de CAS general que conserva identidad aun ante otros cambios identitarios.

**Evidencia:** `recursos.Service.Update`/`UpdateRevision`; `resource_repository_crud.go:147,488`; `UpdateCommand`; búsqueda de FKs/referencias externas.

## E4. Suggested unit

**Clasificación principal: CONFIRMADO.** `Suggested` afecta `NaturalUnitsFor`: devuelve sugeridas primero para que una UI pueda usar la primera como default. `NewResource` no autocompleta unidad y sólo exige `Allowed`; suggested no modifica identidad ni persistencia del Resource. Es metadata con comportamiento de orden editorial, no default aplicado por el dominio.

**Evidencia:** `resource_catalog_query.go:58-85`; `resource_validation.go:23-26,249-257`; comentario explícito “so a UI can default to the first result”.

# F. Presentación

## F1. DisplayName

**Clasificación principal: CONFIRMADO.** PostgreSQL exige y escribe `recursos.display_name`, pero las rutas Create/Update lo llenan con `resource.FamilyCode`, no con `ResourceCatalog.Describe`; las lecturas de Resource no lo seleccionan ni el dominio lo expone. La autoridad pública de presentación es dinámica: `DescribeResource -> recursos.Service.Describe -> ResourceCatalog.Describe`.

No se regenera `display_name` cuando cambian presentation fields/labels; puede quedar stale y, en la práctica, es una columna legacy no consumida por Core. El contrato público no la expone.

**Evidencia:** `resource_repository_crud.go:37-43,147,488`; `resource_presentation.go`; `adapter.go` `DescribeResource`; auditoría §§6.3, 29.14.

## F2. Cambios de Presentation

**Clasificación principal: CONFIRMADO.** Sí: `Describe` toma el snapshot actual en cada llamada. Cambiar tipo, orden o campos de PRESENTACION altera la descripción de Resources históricos sin reescribirlos. OpenSpec exige delegar la “canonical presentation” a la autoridad actual; no promete snapshot histórico.

Que esta mutabilidad fuera deseada como requisito de producto histórico es **NO DETERMINABLE**; el contrato sólo confirma presentación autoritativa dinámica.

**Evidencia:** `recursos.Service.Describe:79-84`; `resource_presentation.go:15-37`; `openspec/specs/resource-master-core/spec.md:85-90`.

## F3. Labels históricos

**Clasificación principal: INFERIDO.** `Describe` emite `value.canonical(...)`; para `CONTROLLED_OPTION` eso es el **código**, no `AttributeOption.Label`. Así, cambiar ROJO label a “Rojo eléctrico” no cambia la descripción de un Resource: seguirá mostrando `ROJO`. El catálogo administrativo sí devuelve el label actualizado para elegir opciones.

No existe presentación histórica de labels ni snapshot del label en valores. La inferencia es que el label nunca participó de la descripción, no que el producto deseara conservarlo.

**Evidencia:** `resource_canonical.go:68-82`; `resource_presentation.go:29-37`; `catalog_loader.go:358-376`; `catalog_admin_kinds.go:702-733`.

# G. Creación y edición de Resource

## G1. Create

**Clasificación principal: CONFIRMADO.** Todos los atributos cuyo modo efectivo es REQUIRED deben enviarse explícitamente. No hay defaults de atributos, autofill ni derivación desde relaciones. Una regla puede volver un atributo forbidden/N/A; entonces debe omitirse. NaturalUnit también es explícita. Suggested sólo ordena opciones de unidad; `ValidOptions` sólo restringe elecciones y no las asigna.

**Evidencia:** `resource_validation.go:27-80`; `resource_catalog_query.go:58-85,121-162`; tests de conductor y canalizaciones de auditoría §14.1.

## G2. Update completo vs patch

**Clasificación principal: CONFIRMADO.** Update es reemplazo completo de scope, NaturalUnit y atributos: el repositorio actualiza parent, borra todos los values e inserta el conjunto recibido dentro de una transacción. OpenSpec de Update lo declara explícitamente full replacement y exige la misma shape completa que Create.

La TUI anterior trabajaba con ficha/editor completo y reenviaba el estado, pero no hay evidencia de una necesidad de negocio que descarte patches para siempre. “Full replace” es contrato legacy documentado; su motivación original es **NO DETERMINABLE**.

**Evidencia:** `resource_repository_crud.go` Update/UpdateRevision; auditoría RM-INV-028; diseño archivado `resource-master-core-write-update:134-135`.

## G3. Atributos no identitarios

**Clasificación principal: NO DETERMINABLE.** El único ejemplo real/semilla confirmado es `diameter_mm`; participa en validación, relación, persistencia y presentación posible, pero no identidad. No se observaron otros bindings no identitarios en los ocho actuales. El modelo permite cualquier tipo/modo con `IdentityParticipates=false`.

La razón de negocio específica de `diameter_mm` no está documentada; es razonable inferir que duplica una equivalencia descriptiva, pero no se eleva a requisito.

**Evidencia:** `resource_catalog.go:44-46`; datos vivos de auditoría §5.3; `TestCanalizacionesNaturalUnitDoesNotParticipateInIdentity` y tests de relaciones.

## G4. Optional + identity

**Clasificación principal: CONFIRMADO.** El modelo permite `ModeOptional` con `IdentityParticipates=true`. Si falta, no agrega componente; si está presente, agrega código+tipo+valor y distingue identidad. No existe binding semilla/vivo con esa combinación ni test end-to-end específico; sí existen registros de test OPTIONAL no identitarios y reglas que retornan OPTIONAL.

**Evidencia:** `resource_validation.go:59-80`; `AttributeMode`; búsquedas en `catalog_mutation_test.go` y fixtures CAS; ausencia en `SeedResourceCatalog`/datos vivos.

## G5. Clase/Familia/Tipo durante update

**Clasificación principal: NO DETERMINABLE.** `UpdateCommand` permite reemplazar el scope y SQL actualiza los tres IDs conservando ResourceID. No se encontró caso de UI/producto que moviera un Resource; las pruebas CAS cambian atributos, no clase. La capacidad produce además el defecto post-commit al cambiar clase.

Es, por evidencia disponible, capacidad técnica del comando, no requisito legacy probado.

**Evidencia:** `UpdateCommand`; `recursos.Service.Update*`; `resource_repository_crud.go:453-525`; auditoría §§6.4, 19.1 RM-INV-044.

# H. Lifecycle de catálogos

## H1. Significado de Active en cada kind

**Clasificación principal: YA CONFIRMADO.** Los 11 kinds poseen lifecycle. En general, inactive retiene la fila para historia y la excluye de nuevas selecciones/writes; no equivale a hard delete. No es perfectamente uniforme: Presentation inactiva sigue siendo usada por `Describe`; binding/rule/relation inactivos bloquean nuevos writes en vez de ser simplemente ignorados; lectura SQL de reglas sí ignora regla inactiva.

**Evidencia:** auditoría §§20.2, 25 y 31; `resource_presentation.go:49-57`; B5/C3 de este documento.

## H2. Reactivación de catálogos

**Clasificación principal: YA CONFIRMADO.** Para cualquier kind, la aplicación crea un snapshot candidato con Active=true y valida coherencia de catálogo completo antes de persistir/publicar. Esto cubre jerarquía Class/Family/Type; definición y bindings; OptionSet/Option/Relation; Unit/Policy; Applicability+Rules; y Presentation mediante constraints/reload según la cobertura real de `Validate`.

No revalida uno por uno todos los Resources afectados. `ReferencedByResources` participa en guards de rename/delete, no en una auditoría masiva de reactivación. Ver auditoría §§8.2, 20.2 y RM-INV-031/032.

**Evidencia:** `catalogo.Service.setActiveRevision`; `ApplyCatalogMutation`; `ResourceCatalog.Validate`; OpenSpec base “Complete catalog lifecycle”.

## H3. Hard-delete

**Clasificación principal: CONFIRMADO.** La intención documentada fue limpiar entradas administrativas creadas por error y nunca usadas; no resolver clutter histórico general, reutilización de códigos ni auditoría. Requiere inactivo, cero dependencias y cero referencias de Resources. `HardDeleteResource` fue deliberadamente excluido porque “in use” no estaba definido.

**Evidencia:** propuesta archivada `resource-master-core-write-harddelete:164-211` (confirmación del usuario); spec base `Conservative guarded hard delete`; auditoría §20.2.

## H4. Dependencias blocking vs no blocking

**Clasificación principal: CONFIRMADO.** `Children.Blocking` se diseñó para desactivación/guía de usuario, pero hard-delete aplica política conservadora y bloquea **cualquier** count, incluso dependencias nominalmente no blocking e históricas. PRESENTACION es el ejemplo declarado no blocking bajo TIPO, pero aun bloquea hard-delete.

La intención está explícita: metadata Blocking no puede relajar eliminación física; los FKs son backstop de carrera.

**Evidencia:** `catalog_record.go:128-134`; diseño de estabilización §Conservative hard delete; spec base líneas 159-189; auditoría §31.2.

# I. Búsqueda

## I1. Filtros tipados

**Clasificación principal: CONFIRMADO.** Cada filtro es igualdad exacta y todos combinan con AND mediante un `EXISTS` por atributo. INTEGER/DECIMAL comparan NUMERIC/entero; Quantity compara magnitud **y** código de unidad sin conversión; boolean/text/option comparan igualdad. Sólo acepta `value_state='SET'`: no hay filtro para N/A ni “atributo ausente”. Un atributo ausente no satisface el EXISTS.

El filtro espera payload canónico; decimal SQL compara por valor, y texto es case-sensitive. Ambigüedad de binding (`scope_count>1`) fuerza camino de integridad, no un match normal.

**Evidencia:** `resource_repository_attributes.go:68-95`; `resource_repository_search.go:84-90`; `SearchCriteria.Filters` comentarios.

## I2. Texto libre

**Clasificación principal: NO DETERMINABLE.** La implementación busca `ILIKE '%texto%'` en `identity_key`, `family.code` y `family.name`. No busca type, option label, `display_name`, códigos/valores individuales, aliases ni keywords. No hay documento que establezca esa terna como necesidad de usuario; parece superficie inicial.

Aliases/keywords existen sólo en CLASE y en listado de catálogo según fields searchable, no en búsqueda de Resources.

**Evidencia:** `resource_repository_search.go`; auditoría §21; `catalog_kind.go` descriptores.

## I3. Scope

**Clasificación principal: NO DETERMINABLE.** Internamente Search sólo admite ACTIVE o INACTIVE. El DTO público enumera ALL, Reader lo acepta, pero bridge lo rechaza explícitamente. La TUI histórica necesitaba descubrir inactivos para reactivación, no hay evidencia de pantalla que mezclara ambos estados en una página.

Por tanto ScopeAll es promesa pública incompleta, no necesidad confirmada.

**Evidencia:** `SearchCriteria.Normalize`; `resourcecore.ScopeAll`; `Adapter.SearchResources`; test `TestAdapter_SearchResourcesAllScopeNotSupported`; auditoría RM-INV-042.

## I4. Orden

**Clasificación principal: NO DETERMINABLE.** El único contrato implementado es `identity_key ASC, id ASC`, elegido para paginación estable. No existen parámetros ni requisitos para ordenar por display name, family/type, fecha, código o atributos.

**Evidencia:** `resource_repository_search.go`; pruebas de boundaries; auditoría §§11.2 y 21.

## I5. Escala prevista

**Clasificación principal: NO DETERMINABLE.** No hay requisito ni estimación de Resources, familias, tipos, opciones o valores por Resource. La base observada tiene 5 Resources, 2 familias, 2 tipos, 54 opciones y 21 values; sólo prueba escala mínima. Los límites de página 50 y carga set-based muestran cautela técnica, no volumen de producto.

**Evidencia:** auditoría §§5.2, 11.2–11.3; ausencia de SLO/capacity plan.

# J. Contrato público

## J1. Filtros de atributos

**Clasificación principal: NO DETERMINABLE.** Existen internamente y se probaron en repositorio; `ResourceQuery` público no tiene el campo. No hay decisión que diga “innecesarios” ni “deliberadamente prohibidos”. La spec general decía “filters”, pero la forma entregada los omitió.

**Evidencia:** `SearchCriteria.Filters`; `resourcecore/queries.go`; bridge; auditoría §9.4.

## J2. Parent filters de catálogo

**Clasificación principal: NO DETERMINABLE.** `CatalogFilter.Parent` permite narrowing por referencias de scope, pero `CatalogQuery` no lo publica. Los descriptores sí exponen ParentKind/ParentField y `RefScopedBy`, señal de UI metadata-driven, pero no hay requirement que confirme que la UI admin pública necesitara filtro parent server-side.

**Evidencia:** `catalog_record.go:115-126`; `resourcecore/queries.go`; auditoría §§9.4, 24.2.

## J3. Dependencies / ReferencedByResources

**Clasificación principal: NO DETERMINABLE.** Son operaciones internas reales usadas por guards de rename/hard-delete, pero Reader no las expone. No existe contrato de preflight UI; la spec prohíbe que preflight autorice delete, aunque podría servir de advertencia.

**Evidencia:** `CatalogAdminRepository`; `catalogo.Service.Dependencies/ReferencedByResources`; spec hard-delete; auditoría §18.2.

## J4. LifecycleResult.Changed

**Clasificación principal: NO DETERMINABLE.** Internamente distingue transición de no-op; el bridge devuelve sólo el Resource/CatalogRecord final. No hay rationale de producto para ocultarlo. La utilidad para UI es evidente, pero no constituye evidencia de requisito.

**Evidencia:** `LifecycleResult`; bridge lifecycle; auditoría §§9.4, 20.

## J5. CatalogDescriptors

**Clasificación principal: CONFIRMADO.** El descriptor interno soporta `Immutable`, `Searchable`, `Guidance`, `AllowCreate`, `SoftDelete` y `Children`; el mapeo público omite esos campos. No fue pérdida accidental desconocida: el contrato público entregado define una proyección reducida y las pruebas de field completeness sólo exigen campos presentes en ambos lados.

La TUI genérica anterior sí usaba descriptors internos para formularios, referencias, create-inline, búsqueda y guards; que el consumidor futuro necesite exactamente todos es **NO DETERMINABLE**.

**Evidencia:** `catalog_kind.go:57-97`; `resourcecore/types.go:7-23`; bridge `mapCatalogDescriptor`; auditoría §§9.4, 24.2, 31.

## J6. Contrato genérico vs contrato de dominio

**Clasificación principal: CONFIRMADO.** La intención metadata-driven es explícita: un `CatalogRecord`+`FieldDescriptor` evita once structs y permite recorrer los 11 kinds genéricamente. La contrapartida observada es un union manual, IDs hash para tablas sin ID, mappings extensos, metadata pública incompleta, reglas especiales de APLICABILIDAD y handlers SQL por kind.

No hay evidencia que cuantifique “molestia de usuario”; las limitaciones técnicas sí están confirmadas.

**Evidencia:** comentarios `catalog_record.go:21-36`; `catalog_kind.go`; diseño estabilización §§DTO model/Complete catalog mutation; auditoría §§18,24,25.

# K. Identificadores y referencias

## K1. ResourceID

**Clasificación principal: NO DETERMINABLE.** El ID está documentado como estable, se usa para update/lifecycle y se publica. El TUI histórico lo retenía durante edición, pero no se encontró módulo externo ni tabla de este repositorio que almacene ResourceID. Los tests externos usan fakes, no consumidores productivos.

**Evidencia:** `Resource.ID`; `resourcecore.Resource.ID`; búsquedas de `resource_id`/`ResourceID`; migración Supplier Master sin FK; auditoría §§16.1, 18.3.

## K2. IDs de catálogo

**Clasificación principal: CONFIRMADO.** Cuatro kinds proyectan `hashtextextended` de clave natural porque sus tablas no tienen BIGSERIAL adecuado. Se exponen públicamente para Get/update/lifecycle, pero documentación advierte que son opacos, potencialmente colisionables y cambiantes ante rename; no deben persistirse como identidad durable por consumers. No se encontró consumer productivo que los almacene.

**Evidencia:** `resourcecore/doc.go:40-49`; auditoría §§4.3,25; handlers de `CONJUNTO_OPCIONES`, `OPCION`, `POLITICA_UNIDAD`, `PRESENTACION`.

## K3. Códigos vs IDs

**Clasificación principal: CONFIRMADO.** Dentro del snapshot y referencias cruzadas, el diseño favorece claves naturales (`CatalogRef.Code`, compuestas con `|`); IDs son identidad de repositorio/target administrativo. Para Resource, `IdentityV1` es identidad durable y ResourceID es estable para mutación. Para catálogo, la estabilidad externa prometida reside en `CatalogDescriptor.IdentityFields`, no en hash ID.

**Evidencia:** `catalog_record.go:5-18,33-36`; `resourcecore/doc.go:40-49`; spec “Durable and opaque identity semantics”.

# L. Concurrencia

## L1. Necesidad real de CAS

**Clasificación principal: NO DETERMINABLE.** OpenSpec exige CAS y prueba carreras de dos conexiones, pero simultáneamente fija un único proceso writer. Los consumidores históricos eran TUI interna y el futuro PI nombrado; no hay evidencia de múltiples editores reales ni incidentes de lost update. CAS fue una decisión preventiva de robustez/contrato, no una necesidad operativa medida.

**Evidencia:** diseño estabilización §§Writer topology, PostgreSQL fixtures; spec §§216-238,264-279; propuestas de write que nombran TUI/PI.

## L2. Scope de revisión

**Clasificación principal: CONFIRMADO.** La revisión es por Resource o por registro padre de catálogo. Reglas comparten revisión del binding APLICABILIDAD. No hay versión global persistida de ResourceCatalog para consumers; `CatalogAuthority` mantiene una versión in-process al publicar snapshot, no expuesta en `resourcecore`.

**Evidencia:** migración 000008; `CatalogAuthority`; `resource_attribute_rules` sin revision; auditoría §§10,17.3.

## L3. No-op y revisión

**Clasificación principal: CONFIRMADO.** El diseño de estabilización deseaba CAS estricto aun en no-op (`design.md:287-290`). El contrato archivado posterior, adaptándose al código/TUI, documentó otra cosa: catalog Deactivate/Reactivate y Resource Reactivate aceptan stale si ya están en target; Resource Deactivate sí verifica CAS. Esa asimetría está probada y fue declarada contrato caller-facing, pero nació de short-circuits preexistentes.

Para v2 debe separarse: **comportamiento legacy confirmado** = asimetría; **contrato deseado anterior contradictorio** = stale siempre conflict; **decisión futura** = abierta, no deducible por “idempotencia”.

**Evidencia:** spec archivada lifecycle líneas 62-92; diseño estabilización líneas 287-290; `catalogo.Service.setActiveRevision`; `recursos.Service.*Revision`; auditoría §20.

# M. Errores

## M1. Categorías realmente consumidas

**Clasificación principal: NO DETERMINABLE.** OpenSpec y tests prueban identidad programática de las 15 categorías y qué operación alcanza cada una. La TUI antigua reaccionaba a errores, pero no queda consumer productivo actual; no hay evidencia de decisiones UI específicas para cada código citado.

**Evidencia:** spec base “Stable public GARFEX errors”; tests `resourcecore/errors_test.go`, bridge category tables; ausencia de TUI/CLI actual.

## M2. Payload estructurado

**Clasificación principal: NO EXISTÍA.** El error público contiene sólo code y mensaje fijo opaco. No hay ExistingResourceID, CurrentRevision, BlockingDependencies, InvalidFields ni AffectedResources; tampoco `Unwrap`. OpenSpec lo diseñó deliberadamente para seguridad, pero no documentó payloads enriquecidos pendientes.

**Evidencia:** `resourcecore/errors.go`; `internal/core/errors.go`; auditoría §§13,23,32.

## M3. Validation errors

**Clasificación principal: NO EXISTÍA.** El dominio genera mensajes internos con atributo/código concreto, pero `internal/core.Map` los reduce al mensaje global `VALIDATION`; la frontera no ofrece field errors. No puede determinarse si la UI los necesitaba: la TUI recibía errores internos antes de la frontera pública y no es evidencia de requerimiento público.

**Evidencia:** `validation(...)` en `resource_validation.go`; `internal/core/errors.go`; `resourcecore.Error`; auditoría §32.

# N. Auditoría, actor y permisos

## N1. Actor

**Clasificación principal: CONFIRMADO.** OpenSpec es explícito: Actor identifica caller (PI/usuario/servicio), es obligatorio en Writer, se adjunta al contexto y llega al seam diagnóstico; Core no lo autentica, autoriza ni valida y no lo persiste/retorna. No es placeholder indeterminado: su intención documentada es atribución diagnóstica, no auditoría durable.

**Evidencia:** spec base “Actor attribution without persistence”; propuesta `resource-master-core-write:28`; `internal/core/diagnostics.go`; bridge.

## N2. Historial de cambios

**Clasificación principal: NO EXISTÍA.** No hay created_by/updated_by, event log, tombstone ni diff histórico. Sólo timestamps técnicos; HardDelete fue confirmado sin trail durable. No se encontró requirement de quién/qué/cuándo más allá del diagnóstico efímero.

**Evidencia:** migraciones 000002–000008; auditoría §12.4; propuesta hard-delete confirmación 5.

## N3. Authorization

**Clasificación principal: NO EXISTÍA.** No hay roles para consulta, creación, catálogo, identidad, lifecycle o hard-delete; `garfex_app` son permisos técnicos amplios y Actor no autoriza. OpenSpec deja autenticación/autorización enteramente al consumer y HardDelete confirmó “no new authorization model”.

**Evidencia:** propuesta Create `Actor attribution`; propuesta hard-delete líneas 171-176/198-203; grants migraciones; auditoría §§12.3,13.3.

# O. Importación/migración del sistema anterior

## O1. Datos que vale la pena preservar

**Clasificación principal: CONFIRMADO.** Datos reales observados: 3 clases, 2 familias, 2 tipos, 8 características, 1 OptionSet, 54 opciones, 9 relaciones, 2 unidades/políticas, 8 bindings+2 reglas, 5 presentation fields, 5 Resources y 21 valores. Las tres clases y el catálogo de cables/tuberías aparecen tanto en semilla como DB; los cinco Resources son datos vivos, no sólo fixture.

“Vale la pena” funcionalmente significa preservar conocimiento de taxonomía, opciones, relaciones, unidad, aplicabilidad, presentación e identidades si se importa; no implica copiar IDs/esquema. El repositorio no distingue ownership de negocio por fila ni calidad de cada registro.

**Evidencia:** auditoría §§5.2–5.4; consultas READ-ONLY registradas allí; `SeedResourceCatalog` sólo como contraste.

## O2. Identidades existentes

**Clasificación principal: NO DETERMINABLE.** Los cinco Resources tienen IdentityV1 válida y restricciones/migraciones buscan preservarla. No se encontró consumer, FK o archivo que demuestre uso fuera del Core. Que sean durables por contrato no prueba adopción externa.

**Evidencia:** auditoría §§5.4,12.1; spec identidad; búsqueda de consumidores.

## O3. Referential consumers

**Clasificación principal: CONFIRMADO.** Supplier Master es independiente: migración 000006 crea sólo Supplier, Branch y Contact; sus únicas FKs son supplier/branch. Dominio, aplicación, PostgreSQL, `suppliercore` y bridge no importan `internal/domain` de Resource ni guardan ResourceID/IdentityV1. No hay otro módulo consumidor persistente presente.

**Evidencia:** `migrations/000006_supplier_master.up.sql:1-93`; `internal/modules/suppliers/*`; `suppliercore/*`; búsqueda de `resource_id`, `ResourceID`, `resourcecore`.

## O4. Compatibilidad

**Clasificación principal: NO DETERMINABLE.** El legacy exige conservar IdentityV1 durante sus propias migraciones y trata ResourceID como estable internamente, pero no documenta razón de producto para importar IDs exactos a v2. Sin consumidores referenciales encontrados, sólo está confirmada la necesidad de no perder significado funcional/datos elegidos; la compatibilidad de identificadores externos sigue abierta.

**Evidencia:** migraciones 000005/000007/000008; spec identidad; O2/O3.

# P. Funcionalidades quizá no detectadas

## P1.

**Clasificación principal: CONFIRMADO.** Capacidades adicionales encontradas:

- consultas editoriales `ActiveClasses`, `FamiliesFor`, `TypesFor`, `AttributesFor`, `OptionsFor`, `ValidOptions`, `NaturalUnitsFor` y `Effective`;
- aliases/keywords y orden de clases; guidance y create-inline metadata interna;
- relación bidireccional al consultar aunque almacenamiento sea dirigido;
- filtros Parent y dependencias/referencias internas;
- copia defensiva y preservación nil/vacío para reglas agregadas;
- coherencia publish-after-commit y latch de writer indeterminado;
- lectura de inactivos/reactivación y hard-delete administrativo conservador.

**NO EXISTÍAN:** importación/exportación masiva, CSV/BC3, duplicación/clonado Core, shortcuts públicos, defaults aplicados automáticamente, HTTP/gRPC, UI actual o composición productiva en este checkout.

**Evidencia:** `resource_catalog_query.go`; `catalog_kind.go`; servicios; `CatalogAuthority`; búsquedas de operaciones/import/export; auditoría §§8–9,18,24–26.

## P2.

**Clasificación principal: CONFIRMADO.** Funcionalidad observable importante para usuario que no debe perderse por reducir el sistema a CRUD:

1. narrowing de opciones relacionadas en ambos sentidos;
2. omisión dinámica de preguntas/presentación por applicability y N/A;
3. unidad sugerida primero, sin imponerla;
4. descripción canónica configurable por tipo con fallback seguro;
5. descubrimiento explícito de inactivos y reactivación validada;
6. errores estables sin fuga SQL;
7. paginación estable y carga set-based;
8. administración genérica de los 11 kinds, incluido agregado APLICABILIDAD+Rules.

**Evidencia:** tests de `resource_catalog_query`, presentación, lifecycle, bridge y PostgreSQL; auditoría §§14,18,21.

## P3.

**Clasificación principal: CONFIRMADO.** Código/comportamiento que no debe elevarse automáticamente a requisito funcional:

- `display_name` persistido pero ignorado y escrito con el código de familia;
- cuatro IDs hash como parche del DTO genérico;
- `numeric_value/unit_id` de Option sin consumidor Core;
- repositorios/servicios legacy paralelos a V2 y `SetActive`/`Delete` aliases;
- create-confirm y lifecycle confirm-read post-commit;
- catálogo V2/latch/composición residual sin composition root actual;
- precedencia Family/Type dependiente del orden del slice;
- casing/whitespace accidental de canonicalización si producto no lo ratifica;
- asimetrías ScopeAll, limit 0, validación de IDs y no-op CAS;
- permiso SQL DELETE de `garfex_app` más amplio que capacidades Core;
- comentarios obsoletos de `resourcecore/doc.go` (“READ only”) contradichos por Writer actual.

Esto no propone reemplazo; evita confundir deuda/accidente con necesidad.

**Evidencia:** auditoría “Discrepancias/Comportamiento muerto/no protegido”; secciones C, D5, F1 y L3 de este documento; `resourcecore.Writer` frente a `doc.go:13-26`.

# Q. Cierre

## Q1 — Funcionalidad imprescindible

**Clasificación principal: YA CONFIRMADO.** Para equivalencia observable, conservar:

- taxonomía Class→Family→Type y 11 clases de catálogo administrables;
- creación, detalle, búsqueda/paginación, edición completa, deactivate/reactivate;
- lifecycle completo de catálogo y hard-delete sólo bajo guards confirmados;
- seis tipos de atributo más N/A, opciones/OptionSets y relaciones;
- applicability ordenada, presentación configurable y unidades/políticas;
- IdentityV1 durable, ResourceID estable para mutación y duplicado por identidad;
- lectura histórica con catálogo inactivo y reactivación contra autoridad actual;
- filtros tipados internos, descripción canónica y opciones válidas para editores;
- errores públicos estables/opacos y copias defensivas;
- atomicidad de Resource update y agregado APLICABILIDAD, CAS/publish coherence donde el contrato lo promete.

No se exige reproducir PostgreSQL, Go, IDs hash ni la forma genérica concreta.

**Evidencia:** auditoría §§1,14–15,18–21,25–26 y matriz RM-INV; spec base completa.

## Q2 — Reglas de negocio imprescindibles

**Clasificación principal: YA CONFIRMADO.** Invariantes semánticas consolidadas:

1. scope coherente Class/Family/Type y jerarquía activa para nuevos writes;
2. binding efectivo único, Type sobre Family en la semántica a decidir explícitamente;
3. payload coincide con tipo; N/A no lleva payload;
4. required presente, forbidden sin SET, primera regla coincidente y fallback conditional conocido;
5. opción pertenece a OptionSet+característica y pares cumplen relaciones;
6. Quantity positiva, unidad activa/permitida y dimensión compatible, sin conversión implícita;
7. identidad incluye scope y atributos efectivos identitarios ordenados, con delimitación no ambigua; NaturalUnit queda fuera;
8. identidad única por clase incluso inactiva;
9. histórico legible; reactivación revalida catálogo e identidad;
10. reemplazo de atributos atómico; stale CAS no escribe;
11. mutación de catálogo valida antes de persistir/publicar; referencias y dependencias protegen rename/delete;
12. APLICABILIDAD padre+rules es agregado atómico.

**Evidencia:** auditoría §19 RM-INV-001–039 y OpenSpec base requirements de lifecycle, CAS, atomic applicability y publication equivalence.

## Q3 — Defectos/deuda que no deben replicarse

**Clasificación principal: CONFIRMADO.** Lista consolidada:

1. reads ordinarios/reload de catálogo sin revision utilizable para iniciar CAS;
2. DB viva migración 4 frente a código que exige 8;
3. update legacy recalcula identidad y CAS la conserva aunque cambien atributos;
4. cambio CAS de clase puede committear y luego devolver error;
5. no-op CAS asimétrico y contradictorio con diseño original;
6. ScopeAll público aceptado y luego rechazado;
7. limit 0 de catálogo devuelve una fila con metadata falsa;
8. pérdidas públicas: filtros tipados, Parent, dependencies, Changed y descriptor metadata;
9. reglas inactivas: write bloqueado, `effective` las usa, SQL histórico las ignora;
10. precedencia Type/Family por orden accidental e inactive override sin fallback;
11. falta de ciclos/dependencias/antecedentes válidos en reglas y ausencia de overlap validation;
12. `ResourceCatalog.Validate` incompleto frente a constraints;
13. Presentation inactiva no filtrada y `display_name` stale/inútil;
14. `numeric_value/unit_id` huérfanos;
15. confirm-reads post-commit no atómicos;
16. dos statements de Search sin snapshot común;
17. IDs negativos validados de forma desigual;
18. actor sin auditoría durable, autorización inexistente y permisos DB más amplios;
19. hardcoded descriptors/handlers y hash IDs complejizan la API genérica;
20. sin E2E real read→revision→primer CAS ni pruebas clave de limit 0/cambio de clase;
21. documentación obsoleta que contradice el Writer actual;
22. no existe refresh cross-process y la topología single-writer depende de composición externa.

**Evidencia:** auditoría §§1.3,9.4–9.6,14.3,19.1,20–22 y listas de discrepancias/deuda; nuevos hallazgos B5,C1,C2,D5,F1,L3,P3.

## Q4 — Funcionalidades inciertas

**Clasificación principal: NO DETERMINABLE.** Deben decidirse como producto/dominio, sin asumir el legacy:

- reglas de casing/Unicode/acentos para texto e identidad;
- vocabulario y mutabilidad de Dimension; conversiones de unidad;
- si NaturalUnit o movimientos de scope alteran identidad;
- precedencia Type/Family y semántica de override inactivo;
- lenguaje de reglas, ciclos, cadenas, no-match y reglas inactivas;
- optional identitario y atributos no identitarios esperados;
- presentación histórica vs dinámica y uso de labels;
- patch vs full replace y movimiento Class/Family/Type;
- filtros/text search/orden/ScopeAll y escala objetivo;
- metadata/dependencies/Changed que debe exponer el contrato;
- durabilidad requerida de ResourceID/IdentityV1 e importación de los cinco registros;
- CAS estricto en no-op, cantidad real de writers y freshness;
- payloads de error/field errors;
- auditoría durable, roles y autorización;
- necesidad de hard-delete de Resource, import/export/clonado/bulk operations;
- cuáles datos legacy son maestros de negocio y cuáles fixtures/ruido.

**Evidencia:** respuestas clasificadas NO DETERMINABLE de G3/G5, I2–I5, J1–J4, K1, L1, M1, O2/O4 y contradicciones documentadas en L3.

# Matriz de trazabilidad de fuentes

| Área | Fuentes principales | Pruebas/escenarios representativos |
|---|---|---|
| Canonicalización/identidad | `resource_canonical.go`, `resource_validation.go`, `resourcecore/values.go`, migraciones 000005/000007 | `TestNewResourceDerivesV1IdentityFromSortedCanonicalParts`, `TestIdentityComponentPreventsDelimiterCollision`, `TestCanonicalNumbers` |
| Applicability/herencia | `resource_types.go`, `resource_catalog_validate.go`, `resource_repository_attributes.go`, `catalog_loader.go`, migración 000003 | `TestResourceAttributeEffective*`, `TestSeedResourceCatalog_UnitNamesAndConditionalRuntimeFixture`, cardinality integration |
| Opciones/unidades/presentación | `resource_catalog_query.go`, `resource_presentation.go`, migraciones 000002/000004 | `TestValidOptionsNarrows*`, `TestDescribeSkipsFieldMarkedNotApplicable`, unit-name migration tests |
| Resource writes/lifecycle | `recursos/service.go`, `resource_repository_crud.go` | canonical writes, lifecycle integration, `TestResourceRepositoryUpdateRevisionAtomicCAS` |
| Catálogo/lifecycle | `catalogo/service.go`, `catalog_mutation.go`, catalog repositories V1/V2 | all-11 lifecycle/CAS/equivalence/dependency-race tests |
| Contrato público | `resourcecore/*`, `internal/bridge/resourcecore/adapter.go`, `internal/core/errors.go` | external-package, copy, Writer shape y bridge category tables |
| Intención documentada | `openspec/specs/resource-master-core/spec.md` y cambios archivados de estabilización/Create/Update/Lifecycle/HardDelete | escenarios GIVEN/WHEN/THEN citados en cada respuesta |
| Persistencia | migraciones 000002–000008 y `internal/postgres` | integración existente inventariada en auditoría §§14/27 |
| Datos observados | `garfex-resource-master-current-state.md` §§5,12,17,29 | consultas READ-ONLY allí registradas |
| Supplier/consumers | migración 000006, `internal/modules/suppliers`, `suppliercore`, bridge supplier | tests Supplier Master; búsqueda sin referencias Resource |

# Índice de respuestas por clasificación principal

La clasificación cuenta una vez por cada uno de los 68 subapartados; una respuesta puede contener hallazgos secundarios con otra clasificación.

| Clasificación | Cantidad | Subapartados |
|---|---:|---|
| CONFIRMADO | 41 | A1–A5; B1–B5; C1–C2; D2–D5; E1–E4; F1–F2; G1–G2, G4; H3–H4; I1; J5–J6; K2–K3; L2–L3; N1; O1, O3; P1–P3; Q3 |
| INFERIDO | 1 | F3 |
| NO EXISTÍA | 4 | M2–M3; N2–N3 |
| NO DETERMINABLE | 16 | G3, G5; I2–I5; J1–J4; K1; L1; M1; O2, O4; Q4 |
| YA CONFIRMADO | 6 | C3; D1; H1–H2; Q1–Q2 |
| **Total** | **68** | A1–Q4 |
