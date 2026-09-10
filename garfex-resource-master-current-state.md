# Estado actual de Recursos Maestros / Materiales Maestros en GARFEX

**Corte auditado:** commit `bf2b777` de la rama `main`

**Base PostgreSQL observada:** `garfex`, PostgreSQL `17.5`, migración `4`, `dirty=false`

**Fecha de observación de la base:** `2026-08-23 01:11:26 UTC`
**Naturaleza del documento:** auditoría factual y READ-ONLY del Core actual

## Convenciones de evidencia

- **Confirmado:** observado directamente en código, migraciones, pruebas o consultas READ-ONLY a PostgreSQL.
- **Inferido:** consecuencia técnica de dos o más hechos confirmados, sin ejecución end-to-end que la demuestre en este corte.
- **No encontrado:** la búsqueda deliberada no localizó la capacidad; no equivale a afirmar que sea imposible fuera de este repositorio.

Las referencias de archivo corresponden al corte indicado. La base viva no fue migrada, reiniciada ni modificada.

---

## 1. Resumen ejecutivo

### 1.1 Veredicto factual

**Confirmado.** El Core contiene un Resource Master genérico con dominio, casos de uso, puertos, adaptadores PostgreSQL, bridge y contrato público `resourcecore`. “Material” no es un agregado ni módulo independiente: es la clase `MATERIAL` dentro de ese Resource Master.

**Confirmado.** El modelo implementa:

- clases, familias y tipos de recurso;
- características tipadas, conjuntos de opciones y relaciones entre opciones;
- aplicabilidad por familia o tipo, reglas condicionales y `NOT_APPLICABLE`;
- unidades naturales y políticas de unidad;
- presentación canónica configurable;
- identidad canónica `v1`;
- creación, consulta, búsqueda, reemplazo, desactivación y reactivación;
- administración genérica de once tipos de catálogo;
- control optimista por revisión (CAS) en código y en la migración 8 de HEAD;
- contrato público de siete lecturas y nueve escrituras graduadas.

### 1.2 Estado operativo observado

| Dimensión | Estado actual | Evidencia |
|---|---|---|
| Dominio y validación | Implementado y cubierto por pruebas unitarias | `internal/domain/*resource*`; pruebas focalizadas exitosas |
| Persistencia legacy | Implementada | `internal/postgres/resource_repository_crud.go`, `catalog_admin_repository.go` |
| Búsqueda paginada | Implementada, parametrizada y con hidratación set-based | `resource_repository_search.go`; prueba de integración existente |
| CAS de recursos | Implementado y conectado al servicio/bridge | `ResourceRepositoryV2`, `UpdateRevision`, lifecycle revision-aware |
| CAS de catálogo | Implementado, pero requiere `CatalogAdminRepositoryV2` | `catalog_admin_repository_v2.go`, `WithCatalogAdminRepositoryV2` |
| Composición productiva | **No encontrada en este repositorio Core** | no hay TUI, CLI ni `main` de producto |
| Base viva | Atrasada respecto de HEAD | migración viva 4; HEAD llega a 8 |
| Contrato público de lectura | Compilable y probado con fakes | `resourcecore.Reader` |
| Contrato público CAS end-to-end | **Incompleto en el estado actual** | lecturas ordinarias exponen revisión 0; escrituras CAS exigen revisión > 0 |

### 1.3 Hallazgos de mayor impacto

1. **Confirmado — deriva de esquema.** La base viva está en migración 4 y no posee columnas `revision`; HEAD agrega esas columnas en `000008_resource_revisions.up.sql`. Las operaciones CAS actuales no son ejecutables contra esa base sin encontrar errores SQL por columna inexistente.
2. **Confirmado — bootstrap CAS público ausente.** `Get`, `SearchPage` y las lecturas legacy de catálogo no seleccionan `revision`. El bridge copia ese cero a `resourcecore.Resource`/`CatalogRecord`, mientras `Writer` exige `ExpectedRevision > 0` para update y lifecycle.
3. **Confirmado — semántica de update divergente.** El update legacy reemplaza `identity_key`; `UpdateRevision` reemplaza scope/atributos pero conserva deliberadamente la identidad anterior, incluso si cambian atributos identitarios. El contrato público usa esta segunda ruta.
4. **Confirmado — catálogo V2 dormant.** El repositorio CAS de catálogo existe y tiene pruebas, pero su propio constructor documenta que no está conectado a una composición productiva; este repositorio no contiene una composición que contradiga ese comentario.
5. **Confirmado — paginación pública de catálogo asimétrica.** `CatalogQuery.Limit == 0` termina solicitando una fila y reportando `HasNext=false`; búsqueda de recursos normaliza el límite no positivo a 50.
6. **Confirmado — scope público no uniforme.** `ScopeAll` funciona para catálogo, pero el bridge lo rechaza explícitamente para búsqueda de recursos.
7. **Confirmado — error posterior al commit posible.** `UpdateRevision` permite cambiar la clase, confirma la transacción y luego intenta recargar por la clase anterior. Esa recarga puede fallar aunque el reemplazo ya haya quedado persistido.

---

## 2. Alcance, método y límites de la auditoría

### 2.1 Alcance inspeccionado

1. modelo de dominio;
2. invariantes y canonicalización;
3. casos de uso de recursos y catálogo;
4. puertos legacy y V2;
5. adaptadores PostgreSQL;
6. contrato público `resourcecore`;
7. bridge público-interno;
8. migraciones 000002–000008;
9. pruebas unitarias y de integración existentes;
10. esquema, datos, permisos y planes de la base viva.

### 2.2 Método

**Confirmado.** La estructura y los caminos de llamada se relevaron primero con CodeGraph. Los detalles no indexados o truncados se verificaron con lecturas focalizadas. PostgreSQL se inspeccionó dentro de transacciones `BEGIN READ ONLY`; se usó `EXPLAIN` sin `ANALYZE` para evitar ejecución con efectos laterales.

Se ejecutaron sólo pruebas focalizadas sin mutar la base viva:

```text
go test ./internal/domain ./internal/app/recursos ./internal/app/catalogo ./resourcecore ./internal/bridge/resourcecore -count=1
go test ./internal/postgres -run 'Test(EncodeAttributeValue|MapRepositoryError|IdentityConflictNever|ClampLimitOffset|ClassifyLoadedCatalog|NormalizeCatalogForParity|AppendActiveFilter)' -count=1
```

Ambos comandos finalizaron correctamente.

### 2.3 Límites

- **Confirmado.** No se ejecutaron pruebas de integración contra la base viva porque varias fixtures limpian recursos, alteran índices o aplican/revierten migraciones.
- **Confirmado.** No se ejecutaron `go build`, migraciones ni comandos destructivos.
- **No encontrado.** No hay interfaz de usuario, CLI, servidor HTTP/gRPC ni composition root productivo en este repositorio. La auditoría termina en el Core y sus adaptadores.
- **No verificado.** No se midió rendimiento con carga representativa. Los planes observados corresponden a cinco recursos y catálogos pequeños.

---

## 3. Arquitectura y fronteras

### 3.1 Mapa de capas

```text
Consumidor externo
  -> resourcecore.Reader / resourcecore.Writer
  -> internal/bridge/resourcecore.Adapter
  -> internal/app/catalogo.Service + internal/app/recursos.Service
  -> puertos de internal/domain
  -> internal/postgres
  -> PostgreSQL
```

**Confirmado.** Las dependencias apuntan hacia el dominio. `internal/domain` no importa PostgreSQL ni el contrato público. El bridge traduce DTOs y errores; `resourcecore` no importa paquetes `internal`, como prueba `TestExternal_NoInternalImports`.

### 3.2 Componentes autoritativos

| Responsabilidad | Autoridad actual |
|---|---|
| Estructura runtime de catálogo | snapshot `domain.ResourceCatalog` cargado desde PostgreSQL |
| Validación de recurso nuevo/reemplazado | `domain.NewResource` |
| Identidad canónica | `internal/domain/resource_validation.go` y `resource_canonical.go` |
| Persistencia | adaptadores en `internal/postgres` |
| Publicación coherente in-process | `domain.CatalogAuthority` |
| Clasificación pública de errores | `internal/core.Map` -> `resourcecore.Error` |

**Confirmado.** `CatalogAuthority.Current` entrega una copia defensiva y una versión in-process. `Publish` reemplaza el snapshot bajo mutex. No se encontró polling, LISTEN/NOTIFY ni recarga automática ante cambios efectuados por otro proceso.

### 3.3 Materiales Maestros

**Confirmado.** No existen `Material`, `MaterialRepository` ni `MaterialService` como agregado vertical separado. Un material es:

```text
Resource{ClassCode: "MATERIAL", FamilyCode: ..., TypeCode: ...}
```

Las otras clases semilla son `MANO_DE_OBRA` y `EQUIPO_HERRAMIENTA`, aunque la base observada sólo tiene familias y tipos cargados bajo `MATERIAL`.

---

## 4. Modelo de dominio y relaciones

### 4.1 Grafo conceptual

```text
ResourceClass
  1 ── * ResourceFamily
          1 ── * ResourceType
          1 ── * ResourceUnitPolicy * ── 1 UnitDefinition
          1 ── * ResourceAttribute ── 1 AttributeDefinition
                    │                     │
                    │                     └── * AttributeOption
                    ├── * AttributeRule
                    └── 0..1 ResourceType (scope específico)

ResourceType 1 ── * PresentationField ── 1 AttributeDefinition

AttributeOption * ── * AttributeOption mediante AttributeOptionRelation

Resource
  * ── 1 ResourceClass / ResourceFamily / ResourceType / UnitDefinition
  1 ── * ResourceAttributeValue ── 1 ResourceAttribute / AttributeDefinition
```

### 4.2 Entidades e identidades

| Concepto | Tipo/tabla | Identidad actual |
|---|---|---|
| Clase | `ResourceClass` / `resource_classes` | ID BIGSERIAL; `code` y `slug` únicos |
| Familia | `ResourceFamily` / `resource_families` | ID; natural `(class_id, code)` |
| Tipo | `ResourceType` / `resource_types` | ID; natural `(family_id, code)` |
| Unidad | `UnitDefinition` / `unit_definitions` | ID; código y símbolo únicos |
| Política | `ResourceUnitPolicy` | clave `(family_id, unit_id)` |
| Característica | `AttributeDefinition` | ID; código global único |
| Aplicabilidad | `ResourceAttribute` | binding único por familia o por tipo |
| Regla | `AttributeRule` | hija ordenada del binding |
| Conjunto | `ResourceOptionSet` | código natural |
| Opción | `AttributeOption` | `(option_set, definition_id, code)` |
| Relación | `AttributeOptionRelation` | par de opciones dentro del set |
| Presentación | `PresentationField` | posición única por tipo |
| Recurso | `Resource` / `recursos` | ID estable + unique `(class_id, identity_key)` |
| Valor | `ResourceAttributeValue` | uno por `(resource_id, resource_attribute_id)` |

### 4.3 IDs sintéticos de catálogo

**Confirmado.** `CONJUNTO_OPCIONES`, `OPCION`, `POLITICA_UNIDAD` y `PRESENTACION` no poseen un BIGSERIAL propio compatible con el DTO genérico. Sus handlers de catálogo producen `int64` con `hashtextextended()` sobre claves naturales.

**Inferido.** En esos cuatro kinds el ID público es una proyección determinista de la clave natural, no una identidad persistida independiente. Un cambio de los componentes naturales cambia el ID proyectado; el repositorio resuelve operaciones a partir de esa proyección y sus claves.

---

## 5. Catálogo maestro y datos observados

### 5.1 Once kinds administrables

`CatalogRegistry` registra:

1. `CLASE`;
2. `FAMILIA`;
3. `TIPO`;
4. `CARACTERISTICA`;
5. `CONJUNTO_OPCIONES`;
6. `OPCION`;
7. `RELACION_OPCIONES`;
8. `UNIDAD`;
9. `POLITICA_UNIDAD`;
10. `APLICABILIDAD`;
11. `PRESENTACION`.

### 5.2 Snapshot vivo

| Tabla | Filas |
|---|---:|
| `resource_classes` | 3 |
| `resource_families` | 2 |
| `resource_types` | 2 |
| `attribute_definitions` | 8 |
| `resource_option_sets` | 1 |
| `attribute_options` | 54 |
| `attribute_option_relations` | 9 |
| `unit_definitions` | 2 |
| `resource_unit_policies` | 2 |
| `resource_attributes` | 8 |
| `resource_attribute_rules` | 2 |
| `resource_type_presentation_fields` | 5 |
| `recursos` | 5 |
| `resource_attribute_values` | 21 |

### 5.3 Aplicabilidad viva

**CABLE / CONDUCTORES**

| Orden | Característica | Modo base | Identitaria |
|---:|---|---|---|
| 0 | `conductor_material` | REQUIRED | sí |
| 1 | `gauge` | REQUIRED | sí |
| 2 | `insulation` | REQUIRED | sí |
| 3 | `color` | CONDITIONAL | sí |
| 4 | `voltage` | CONDITIONAL | sí |

Para `insulation = DESNUDO`, `color` y `voltage` pasan a `FORBIDDEN`, dejan de participar en identidad y se marcan `NOT_APPLICABLE`.

**TUBERIA / CANALIZACIONES**

| Orden | Característica | Modo | Identitaria |
|---:|---|---|---|
| 0 | `tipo` | REQUIRED | sí |
| 1 | `diameter_inch` | REQUIRED | sí |
| 2 | `diameter_mm` | REQUIRED | no |

Existen nueve relaciones bidireccionalmente consultables entre diámetros, desde `1/2" -> 13 mm` hasta `4" -> 100 mm`.

### 5.4 Recursos vivos

**Confirmado.** Se observaron cinco recursos activos: tres cables y dos tuberías. Los 21 valores estaban en estado `SET`; no se observaron valores `NOT_APPLICABLE` persistidos en esas cinco filas.

---

## 6. Canonicalización, identidad y presentación

### 6.1 Construcción canónica

`domain.NewResource`:

1. canonicaliza clase, familia, tipo y unidad;
2. resuelve el scope contra el catálogo activo;
3. rechaza características duplicadas o desconocidas;
4. canonicaliza el payload según su tipo;
5. aplica reglas condicionales;
6. valida required/optional/forbidden;
7. valida opciones y relaciones;
8. valida política de unidad;
9. ordena partes identitarias;
10. deriva `IdentityKey`.

### 6.2 Formato de identidad v1

```text
v1|
  len-bytes:CLASS
  len-bytes:FAMILY
  len-bytes:TYPE
  [len:attribute-code][len:value-type][len:canonical-value]...
```

**Confirmado.** Los componentes se delimitan por longitud UTF-8, evitando colisiones por separadores. Participan clase, familia, tipo y cada valor efectivo marcado como identitario. No participan el ID, la unidad natural, el estado activo ni el nombre de presentación.

**Confirmado.** La restricción `recursos_identity_key_v1` de migración 7 exige prefijo `v1|`. La migración 5 construye un mapa legacy-v1 y aborta ante colisiones canónicas, scopes inválidos o aplicabilidad superpuesta.

### 6.3 Presentación

`ResourceCatalog.Describe` usa los `PresentationField` del tipo en orden. Omite ausentes y `NOT_APPLICABLE`; si no hay campos devuelve el nombre del tipo; si el tipo no existe devuelve cadena vacía.

### 6.4 Dos semánticas de actualización

| Ruta | Identidad después del reemplazo |
|---|---|
| `Service.Update` -> repositorio `Update` | escribe la identidad recién derivada |
| `Service.UpdateRevision` -> repositorio `UpdateRevision` | conserva la identidad almacenada |

**Confirmado.** `TestResourceRepositoryUpdateRevisionAtomicCAS` cambia todos los atributos identitarios de un conductor y exige explícitamente que la identidad permanezca igual. Esto no es una mera omisión no probada: es el comportamiento esperado por la prueba existente.

**Inferido.** Si ese recurso luego se desactiva, la reactivación canónica puede fallar: `reactivationCandidate` vuelve a derivar la identidad desde los atributos reemplazados y exige igualdad exacta con la identidad almacenada.

**Confirmado.** `UpdateRevision` captura `classCode` e `identityKey` antes de actualizar, permite reemplazar `class_id`, confirma la transacción y llama `getWithRevision` con las claves anteriores. Si cambia la clase, `Get(oldClass, oldIdentity)` ya no encuentra la fila. El caller recibe error después del commit; la prueba de integración existente cambia atributos, pero no clase.

---

## 7. Invariantes de validación

### 7.1 Estructura de catálogo

- `ResourceCatalog.Validate` comprueba clases, familias, referencias de tipo/binding/option-set, reglas, unidades, políticas con unidad conocida y jerarquía activa;
- no comprueba por sí mismo duplicados de tipo, `suggested => allowed`, estructura de opciones/relaciones ni aplicabilidad/posición de presentación; esas garantías descansan en constraints PostgreSQL y, para la semilla conocida, en pruebas de consistencia;
- los códigos son únicos en su scope correspondiente por constraints; dominio comprueba explícitamente clase y familia;
- una familia activa no puede depender de una clase inactiva;
- un tipo activo no puede depender de una familia inactiva;
- `ModeConditional` exige reglas y un modo no condicional no puede portar reglas;
- las opciones relacionadas deben pertenecer al conjunto y a las características correctas;
- una unidad sugerida debe estar permitida;
- las posiciones de presentación no se duplican por tipo.

### 7.2 Escritura de recursos

- toda escritura parte de un `Resource` canónico; `ValidateForPersistence` rechaza valores forjados;
- clase, familia, tipo, unidad y política deben estar activos;
- cada valor debe resolver exactamente un binding efectivo;
- valores required deben existir;
- valores forbidden no se aceptan como `SET`;
- `NOT_APPLICABLE` no puede transportar payload;
- opciones controladas deben usar códigos oficiales del option set;
- cantidades requieren unidad activa y dimensión compatible;
- relaciones restringen pares válidos;
- la cantidad persistida debe coincidir con la cantidad esperada.

### 7.3 Lectura histórica

**Confirmado.** `HydrateResource` valida forma e identidad v1, pero no exige que las referencias de catálogo sigan activas. Esto permite leer recursos históricos luego de desactivar estructura. La reactivación sí vuelve a validar contra el catálogo actual.

---

## 8. Casos de uso internos

### 8.1 Recursos

| Método | Comportamiento actual |
|---|---|
| `Get(class, identity)` | devuelve activo o inactivo; carga atributos efectivos |
| `Search(criteria)` | wrapper de `SearchPage`, sin metadatos |
| `SearchPage(criteria)` | activos por defecto o inactivos explícitos; máximo 50 |
| `Describe(resource)` | presentación canónica desde snapshot actual |
| `Create(command)` | canonicaliza y persiste transaccionalmente |
| `Update(command)` | reemplazo completo legacy; recalcula identidad |
| `Deactivate(id)` | soft transition idempotente |
| `Reactivate(id)` | revalida catálogo e identidad antes de activar |
| `UpdateRevision(command, rev)` | reemplazo completo CAS; preserva identidad |
| `DeactivateRevision(id, rev)` | CAS idempotente; incrementa sólo si cambia |
| `ReactivateRevision(id, rev)` | CAS + identidad + referencias activas |
| `Delete(id)` | alias de `Deactivate`; no elimina físicamente |

**No encontrado.** No existe hard-delete de recurso en el servicio, puerto V2, adaptador PostgreSQL ni contrato público.

### 8.2 Catálogo

| Método | Legacy | V2/CAS |
|---|---:|---:|
| List/Get/Dependencies/ReferencedByResources | sí | lectura común legacy |
| Create | sí | ruta V2 si fue configurada |
| Update | sí | `UpdateRevision` |
| Deactivate/Reactivate | sí | revision-aware |
| Hard delete | sí | `HardDeleteRevision` |

**Confirmado.** El servicio serializa mutaciones con mutex, valida una copia del snapshot antes de persistir y publica el snapshot sólo después de éxito.

**Confirmado.** Sin `repoV2`, los métodos V2 devuelven `ErrCatalogAdminRepositoryV2Unavailable`. No se encontró una composition root en este repositorio que invoque `WithCatalogAdminRepositoryV2`.

---

## 9. Contrato público `resourcecore` y bridge

### 9.1 Lecturas públicas

`ReadCapabilities` expone siete operaciones:

1. `ActiveClasses`;
2. `CatalogDescriptors`;
3. `ListCatalog`;
4. `GetCatalog`;
5. `SearchResources`;
6. `GetResource`;
7. `DescribeResource`.

### 9.2 Escrituras públicas

`WriteCapabilities` expone nueve operaciones:

1. `CreateCatalog`;
2. `CreateResource`;
3. `UpdateCatalog`;
4. `UpdateResource`;
5. `DeactivateCatalog`;
6. `ReactivateCatalog`;
7. `DeactivateResource`;
8. `ReactivateResource`;
9. `HardDeleteCatalog`.

### 9.3 Validación de frontera

**Confirmado.** `Reader` y `Writer` validan forma básica y copian defensivamente maps/slices. `Actor` es obligatorio para escrituras y viaja como metadata de contexto; no es parámetro de negocio ni se persiste.

Las operaciones CAS exigen `ExpectedRevision > 0`. Para ID existe una asimetría: Reader y servicio de recursos exigen positivo, pero los validadores públicos de Writer y varios métodos de catálogo sólo rechazan cero; un ID negativo puede alcanzar catálogo/repositorio y terminar como not-found. La validación semántica de contenido permanece en dominio/aplicación.

### 9.4 Query-completeness y pérdidas

| Capacidad interna | Contrato público | Estado |
|---|---|---|
| filtros de recurso por atributos tipados (`SearchCriteria.Filters`) | `ResourceQuery` no tiene filtros de atributos | pérdida confirmada |
| scope activo/inactivo | disponible | completo |
| scope all de catálogo | disponible | completo |
| scope all de recursos | tipo público lo admite; bridge lo rechaza | incompleto explícito |
| parent filters de catálogo | `CatalogQuery` no los expone | pérdida confirmada |
| dependencias y referencias de catálogo | no expuestas por Reader | pérdida confirmada |
| resultados `LifecycleResult.Changed` | Writer devuelve sólo `Resource`/`CatalogRecord` | pérdida confirmada |
| revisión persistida | DTO la posee | no poblada por lecturas ordinarias |

### 9.5 Bootstrap CAS

**Confirmado.** `resourceRepository.Get` y `SearchPage` no seleccionan `recursos.revision`; sólo `getWithRevision`, posterior a una operación CAS, la carga. Las lecturas legacy de catálogo tampoco proyectan `revision`. Por ello:

```text
Get/Search/Create-confirm públicos -> Revision 0
Update/Lifecycle públicos          -> ExpectedRevision > 0 obligatorio
```

El bridge de `CreateResource` realiza un create-confirm read no transaccional mediante `Get`, por lo que también devuelve revisión 0. El comentario limita esa confirmación a una topología aprobada de un único writer, pero esa topología no se compone en este repositorio.

### 9.6 Paginación pública de catálogo

**Confirmado.** `ListCatalog` transforma `q.Limit` en `q.Limit+1`. Para `Limit=0` envía 1 al repositorio; `buildCatalogPage` después fuerza `HasNext=false` y devuelve esa fila. No existe normalización pública equivalente a `SearchCriteria.Normalize`.

---

## 10. Lifecycle y control optimista

### 10.1 Recursos legacy

`Deactivate` y `Reactivate` bloquean la fila con `FOR UPDATE`. Un estado ya alcanzado es no-op. Reactivar verifica identidad esperada, cadena activa y ausencia de conflicto antes del commit.

### 10.2 Recursos V2

- revisión esperada no cero;
- `SELECT ... FOR UPDATE` serializa competidores;
- revisión distinta -> `ErrResourceRevisionConflict`;
- no-op idempotente -> revisión sin incremento;
- transición real -> `revision = revision + 1`;
- update de parent y reemplazo de atributos están en una transacción;
- reactivación no regenera identidad.

### 10.3 Catálogo V2

**Confirmado.** Las once tablas padre reciben revisión en migración 8. `resource_attribute_rules` no tiene revisión propia: pertenece al agregado `APLICABILIDAD` padre. Los handlers CAS distinguen no encontrado de revisión obsoleta mediante consulta explícita, no mediante parsing de mensajes.

### 10.4 Deriva viva

**Confirmado.** La base viva no contiene ninguna columna `revision`. El código actual sí las consulta en loader V2, CAS de recursos y CAS de catálogo. Es una incompatibilidad concreta entre binario HEAD y base observada para esas rutas.

---

## 11. Persistencia PostgreSQL

### 11.1 Escrituras de recurso

```text
Create/Update
  -> ValidateForPersistence
  -> BEGIN
  -> resolver scope/unidad/política activos
  -> INSERT o UPDATE recursos
  -> persistAttributeValue por valor
  -> verifyAttributeCount
  -> COMMIT; rollback diferido ante cualquier error
```

**Confirmado.** Todos los valores se parametrizan. `resolveAttributeTarget` exige exactamente un binding. `Update` y `UpdateRevision` realizan reemplazo completo, no merge.

### 11.2 Lecturas y búsqueda

`SearchPage`:

- normaliza límite/offset;
- combina condiciones con AND;
- usa parámetros para scope, texto y filtros;
- ordena por `identity_key, id`;
- solicita `limit+1`;
- detecta ambigüedad de binding efectivo;
- carga todos los atributos de la página con una segunda consulta set-based.

La prueba de integración existente verifica 1/10/50 filas con dos consultas, boundaries de página, fallback familia/tipo y paridad Search/Get.

### 11.3 Planes observados

| Consulta | Plan vivo |
|---|---|
| lookup `(class_id, identity_key)` con subconsultas de muestra | `Seq Scan` sobre `recursos`; tabla de 5 filas |
| búsqueda textual `ILIKE '%...%'` | scans secuenciales + hash join + sort |
| listado de clases activas | scan secuencial + sort |

**No verificado.** Estos planes no permiten concluir comportamiento a escala. El `ILIKE` con comodín inicial no mostró uso de índice en el plan observado.

### 11.4 Seguridad SQL

**Confirmado.** Inputs de negocio se pasan como parámetros. Las interpolaciones de tabla/columna se limitan a mapas internos de kinds o a `revisionTables`; límite/offset son enteros, no texto crudo. No se encontró SQL construido con identificadores aportados por el consumidor.

---

## 12. Esquema y datos de la base viva

### 12.1 Versión

```text
PostgreSQL:       17.5-bookworm
schema_migrations: version=4, dirty=false
tablas públicas:  15
```

**Confirmado.** La migración 5 agregaría `resource_integrity_identity_map`; la 7 impondría identidad v1 y la 8 agregaría revisiones. Ninguna aparece aplicada según `schema_migrations` ni según columnas/tablas observadas.

**Observación de consistencia.** Los cinco recursos vivos ya tienen identidad con prefijo `v1|` aunque la restricción de migración 7 no está aplicada. El dato cumple el formato por práctica de escritura, no por constraint vivo.

### 12.2 Constraints relevantes de HEAD

- unique `(class_id, identity_key)` para recursos, incluyendo inactivos;
- integridad compuesta clase-familia-tipo;
- checks de unión tipada en valores;
- `suggested => allowed` en políticas;
- índices únicos parciales para bindings por familia o tipo;
- identidad `LIKE 'v1|%'` desde migración 7;
- revisión `BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)` desde migración 8.

### 12.3 Permisos observados

**Confirmado.** `garfex_admin` posee privilegios administrativos completos. `garfex_app` tiene `SELECT`, `INSERT`, `UPDATE` y `DELETE` sobre las tablas Resource Master relevadas, incluida `recursos` y la estructura catalogográfica.

**Confirmado.** Los guards de aplicación no sustituyen permisos de base: la identidad runtime puede ejecutar DELETE físico si usa SQL fuera de los repositorios previstos.

### 12.4 Auditoría de negocio

**No encontrado.** No se localizaron columnas `created_by`/`updated_by`, tabla de eventos de Resource Master ni persistencia de `Actor`. Sí existen marcas temporales técnicas en varias tablas.

---

## 13. Errores, seguridad y observabilidad

### 13.1 Quince categorías públicas

`INVALID_ARGUMENT`, `NOT_FOUND`, `DUPLICATE`, `INVALID_REFERENCE`, `VALIDATION`, `INTEGRITY`, `IDENTITY_CONFLICT`, `INVALID_LIFECYCLE`, `REACTIVATION_IMPOSSIBLE`, `INVALID_CATALOG`, `IN_USE`, `IMMUTABLE_CODE`, `CONFLICT`, `UNAVAILABLE`, `INTERNAL`.

**Confirmado.** `internal/core.Map` clasifica con `errors.Is` y precedencia explícita. El error neutral/público no implementa `Unwrap`, por lo que no filtra la causa interna.

### 13.2 Traducción PostgreSQL

Los repositorios traducen SQLSTATE/constraint a sentinels de duplicate, reference, integrity, in-use o not-found. Los conflictos CAS se distinguen mediante la revisión leída bajo lock.

### 13.3 Contexto y actor

**Confirmado.** Los métodos reciben `context.Context`. El bridge agrega `Actor` al contexto para diagnóstico. **No encontrado:** autorización por actor, tenant, autenticación, rate limiting o auditoría persistente.

### 13.4 Logging

**Confirmado.** El repositorio define configuración de logging por `GARFEX_LOG_LEVEL`, pero las capacidades Resource Master auditadas no contienen una política propia de auditoría funcional. Este documento no evaluó una interfaz de delivery porque no existe en el checkout.

---

## 14. Casos representativos y cobertura de pruebas

### 14.1 Catorce casos trazados

| Caso | Escenario exacto | Camino y resultado actual | Evidencia |
|---:|---|---|---|
| 1 | recurso simple | `NewResource` canonicaliza scope/unidad, valida los mínimos requeridos y deriva identidad `v1`; `Create` persiste padre y valores en una transacción | `resource_validation.go:11-92`; `TestConductorCatalogCreatesValidResources`; `TestResourceRepositoryIntegration` |
| 2 | recurso con varios atributos | cada código se acepta una sola vez, se canonicaliza por `AttributeValueType`, se ordena y se persiste con cardinalidad final verificada | `resource_validation.go:27-89`; `resource_repository_crud.go:52-58,225-258`; `TestEncodeAttributeValuePreservesTypedPayloads` |
| 3 | atributo identitario | un valor efectivo con `IdentityParticipates=true` agrega código, tipo y valor canónico a `IdentityKey` | `resource_validation.go:52-89`; `TestNewResourceDerivesV1IdentityFromSortedCanonicalParts` |
| 4 | atributo no identitario | el valor se valida y persiste, pero no integra identidad; `diameter_mm` es el ejemplo vivo | `resource_catalog.go:46`; `TestCanalizacionesNaturalUnitDoesNotParticipateInIdentity` prueba además la exclusión de unidad natural |
| 5 | característica condicional | las reglas se evalúan en orden; si ninguna aplica, un atributo `CONDITIONAL` es inválido; `insulation=DESNUDO` vuelve `color`/`voltage` prohibidos | `resource_validation.go:61-75`; `TestSeedResourceCatalog_UnitNamesAndConditionalRuntimeFixture` |
| 6 | `NOT_APPLICABLE` | se representa con sentinel de dominio y `value_state`; no admite payload y la presentación lo omite | `resource_types.go:67-71`; `resource_repository_codec.go:27-33`; `TestEncodeAttributeValueRejectsNotApplicableWithPayload`; `TestDescribeSkipsFieldMarkedNotApplicable` |
| 7 | duplicado | `UNIQUE (class_id, identity_key)` cubre activos e inactivos; SQLSTATE `23505` se traduce a duplicado salvo la cardinalidad de valores | migración 000002:132-149; `mapRepositoryError`; `TestCanalizacionesDuplicateIdentity` |
| 8 | inactivo | sigue siendo legible por `Get`; la búsqueda exige scope inactivo explícito; nueva escritura/reactivación revalida catálogo activo | `resource_repository_crud.go:66-95`; `resource_repository_search.go:66-70`; `TestRehydrateResourcePreservesHistoryAfterCatalogDeactivation` |
| 9 | dependencia bloquea eliminación | hard-delete de catálogo exige target inactivo, dependencias en cero y ninguna referencia de recursos; FK es backstop | `catalogo.Service.Delete`/`buildV2DeleteCandidate`; `TestServiceDeleteConservativeGuards`; `TestCatalogConcurrentDependencyDeleteRaceIntegration` |
| 10 | lifecycle válido | activo→inactivo e inactivo→activo son válidos; CAS incrementa revisión exactamente una vez si cambia estado | `setLifecycleRevision`; `TestResourceRepositoryLifecycleIntegration` |
| 11 | lifecycle inválido | hard-delete de catálogo activo devuelve `INVALID_LIFECYCLE`; reactivación incompatible devuelve `REACTIVATION_IMPOSSIBLE` | `catalogo.Service.Delete`; `recursos.Service.reactivationCandidate`; `TestLifecycleInvalidHardDeleteOfActiveTargetIsDistinctFromInvalidCatalog` |
| 12 | conflicto concurrencia | dos writers desde revisión 10 bloquean la misma fila; el primero termina en 11 y el segundo observa revisión distinta y devuelve `CONFLICT`; no hay retry automático | `resource_repository_crud.go:455-497`; `TestResourceRepositoryConcurrentRevisionRaceIntegration` |
| 13 | catálogo con options | `CONJUNTO_OPCIONES` + `OPCION` delimitan códigos controlados por conjunto y característica; opciones inactivas se excluyen | `catalog_kind.go:218-244`; `TestResourceCatalog_OptionsFor_NarrowsByNamedOptionSet`; `TestControlledOptionsRequireOfficialCodes` |
| 14 | catálogo con referencia a otro catálogo | `FieldRef`, `RefKind` y `RefScopedBy` expresan referencias naturales; bridge exige `Reference.ID==0` y transporta `Kind+Code` | `catalog_kind.go:57-70,150-313`; `adapter.go:489-540`; `TestAdapter_CatalogDescriptorsIncludesRefScopedByAndEnumValues` |

### 14.2 Cobertura observada

Conteo reproducible de funciones `Test*` en los paquetes auditados: 130 en `internal/domain`, 22 en `internal/app/recursos`, 36 en `internal/app/catalogo`, 39 en `internal/postgres`, 45 en `resourcecore` y 66 en `internal/bridge/resourcecore`: **338 pruebas**. El conteo es de funciones, no de subtests.

El subconjunto ejecutado en esta auditoría pasó. Las pruebas de integración existen pero no se ejecutaron contra la base viva.

### 14.3 Vacíos de prueba relevantes

**No encontrado.** No se encontró una prueba end-to-end real que componga `resourcecore.Reader` + bridge + PostgreSQL, lea una revisión no cero y luego complete el primer write CAS.

**No encontrado.** No se encontró prueba pública para `CatalogQuery{Limit: 0}` a través del bridge real.

**Confirmado.** La prueba CAS de update afirma la identidad inmutable aun cambiando todos los atributos identitarios; no prueba una desactivación/reactivación posterior de ese mismo recurso actualizado.

---

## 15. Inventario de invariantes, matriz de cobertura y conclusión

### 15.1 Inventario histórico consecutivo

Esta tabla conserva la numeración original para trazabilidad. La clasificación autoritativa está en §19: RM-INV-001–039 son invariantes ejecutables; RM-INV-040–044 son expectativas incumplidas y no deben interpretarse como reglas aplicadas por el sistema.

| ID | Invariante actual | Capa principal | Estado |
|---|---|---|---|
| RM-INV-001 | una familia pertenece a una clase existente | dominio + FK | Confirmado |
| RM-INV-002 | un tipo pertenece a familia y clase coherentes | dominio + FK compuesta | Confirmado |
| RM-INV-003 | códigos de familia son únicos dentro de clase | dominio + unique | Confirmado |
| RM-INV-004 | códigos de tipo son únicos dentro de familia | dominio + unique | Confirmado |
| RM-INV-005 | hijos activos no dependen de padres inactivos | dominio | Confirmado |
| RM-INV-006 | unidad sugerida implica unidad permitida | dominio + check | Confirmado |
| RM-INV-007 | característica define un único tipo de payload | dominio + checks SQL | Confirmado |
| RM-INV-008 | binding aplica por familia o tipo y no por scope ambiguo | dominio + índices parciales + loader | Confirmado |
| RM-INV-009 | modo CONDITIONAL equivale a presencia de reglas | dominio | Confirmado |
| RM-INV-010 | reglas activas determinan modo/identidad/N/A efectivos | dominio | Confirmado |
| RM-INV-011 | opción pertenece al option set y característica correctos | dominio + FK | Confirmado |
| RM-INV-012 | relaciones unen opciones válidas del mismo conjunto | dominio + FK/checks | Confirmado |
| RM-INV-013 | campos de presentación son aplicables y no duplican posición | dominio + unique | Confirmado |
| RM-INV-014 | escritura sólo acepta Resource canónico | dominio | Confirmado |
| RM-INV-015 | required presente y forbidden ausente como SET | dominio | Confirmado |
| RM-INV-016 | NOT_APPLICABLE no transporta payload | dominio + codec/SQL | Confirmado |
| RM-INV-017 | cantidad usa unidad activa y compatible | dominio + repositorio | Confirmado |
| RM-INV-018 | identidad v1 incluye clase/familia/tipo y valores identitarios ordenados | dominio | Confirmado |
| RM-INV-019 | delimitación por longitud evita colisiones de separador | dominio + función SQL migración 5 | Confirmado |
| RM-INV-020 | unidad natural no participa en identidad | dominio | Confirmado |
| RM-INV-021 | identidad es única por clase también para inactivos | unique SQL | Confirmado |
| RM-INV-022 | lectura histórica no depende de actividad actual del catálogo | hidratación | Confirmado |
| RM-INV-023 | reactivación revalida catálogo e identidad actual | aplicación + repositorio | Confirmado |
| RM-INV-024 | lifecycle es idempotente | dominio + repositorio | Confirmado |
| RM-INV-025 | transición CAS real incrementa revisión una vez | repositorio V2 | Confirmado en código/pruebas; no en DB viva |
| RM-INV-026 | no-op CAS conserva revisión | repositorio V2 | Confirmado en código/pruebas; no en DB viva |
| RM-INV-027 | revisión obsoleta no produce write parcial | repositorio V2 | Confirmado en pruebas de integración existentes |
| RM-INV-028 | update reemplaza el conjunto completo de atributos atómicamente | repositorios | Confirmado |
| RM-INV-029 | búsqueda devuelve ganador efectivo único por característica | SQL + integrity error | Confirmado |
| RM-INV-030 | búsqueda pagina con orden estable y limit+1 | dominio + SQL | Confirmado |
| RM-INV-031 | mutación de catálogo se valida antes de publicar | aplicación | Confirmado |
| RM-INV-032 | publicación de catálogo ocurre después de persistir | aplicación | Confirmado |
| RM-INV-033 | referencias/códigos protegidos no se eliminan o renombran libremente | aplicación + FK | Confirmado |
| RM-INV-034 | errores públicos no exponen causa interna | core + resourcecore | Confirmado |
| RM-INV-035 | colecciones públicas se copian defensivamente | resourcecore | Confirmado |
| RM-INV-036 | hard-delete público existe sólo para catálogo | contrato público | Confirmado |
| RM-INV-037 | update legacy deriva y persiste nueva identidad | aplicación + repositorio legacy | Confirmado |
| RM-INV-038 | update CAS conserva identidad almacenada | repositorio V2 + prueba | Confirmado |
| RM-INV-039 | writes CAS públicos requieren ExpectedRevision no cero | resourcecore | Confirmado |
| RM-INV-040 | lecturas ordinarias deberían aportar la revisión necesaria para CAS | contrato implícito read-modify-write | **No satisfecho** |
| RM-INV-041 | base ejecutora debe tener las columnas consultadas por CAS | esquema/runtime | **No satisfecho en DB viva** |
| RM-INV-042 | scope ALL de recursos debería representar ambos estados si se acepta en el DTO | bridge | **No satisfecho; se rechaza explícitamente** |
| RM-INV-043 | límite cero de catálogo debería tener semántica de página coherente | bridge | **No satisfecho** |
| RM-INV-044 | una escritura que informa error no debería haber confirmado una mutación no observable por su respuesta | repositorio V2 | **No satisfecho al cambiar clase** |

### 15.2 Matriz resumida de cobertura

| Invariantes | Dominio unit | App unit | PostgreSQL unit | PostgreSQL integración | Público/bridge |
|---|:---:|:---:|:---:|:---:|:---:|
| RM-INV-001–013 catálogo estructural | sí | parcial | parcial | sí, existente | parcial |
| RM-INV-014–020 validación/identidad | sí | sí | sí | sí, existente | shape-only |
| RM-INV-021–024 duplicado/histórico/lifecycle | sí | sí | parcial | sí, existente | parcial |
| RM-INV-025–028 CAS/atomicidad | parcial | sí | parcial | sí, existente | fakes |
| RM-INV-029–030 búsqueda/paginación | sí | sí | sí | sí, existente | parcial |
| RM-INV-031–033 catálogo write/coherencia | sí | sí | parcial | sí, existente | fakes |
| RM-INV-034–039 frontera pública | n/a | parcial | n/a | no end-to-end | sí |
| RM-INV-040–044 brechas actuales | no | no | no | no | evidencia negativa confirmada |

### 15.3 Consistencia cruzada

| Afirmación | Código HEAD | Migraciones HEAD | Base viva | Resultado |
|---|---:|---:|---:|---|
| identidad v1 | sí | sí, 000005/000007 | datos sí; constraint no | parcial vivo |
| revisiones | sí | sí, 000008 | no | deriva crítica |
| CAS de recursos | sí | requiere 000008 | no ejecutable | implementado, no operativo en vivo |
| CAS de catálogo | sí | requiere 000008 | no | implementado, dormant/no operativo |
| contrato público CAS | sí | compatible con 000008 | revisiones ausentes y reads en cero | incompleto |
| hard-delete de recurso | no | FK permitiría SQL directo según dependencias | permiso DELETE existe | no es caso de uso Core |

### 15.4 Conclusión

**Confirmado.** GARFEX posee un Resource Master de dominio considerablemente completo y probado, con Materiales como una clase de recurso. La canonicalización, reglas condicionales, identidad v1, persistencia transaccional, búsqueda set-based, lifecycle y catálogo genérico son capacidades reales del Core.

**Confirmado.** El estado actual no equivale, sin matices, a una superficie pública CAS operativa: la base observada está cuatro migraciones detrás de HEAD; las lecturas públicas no entregan las revisiones requeridas para iniciar un write CAS; el catálogo V2 no está compuesto; el update CAS conserva una identidad potencialmente divergente de sus atributos; un cambio CAS de clase puede confirmar y luego informar error; y existen asimetrías públicas de filtros, scope y paginación.

**No encontrado.** No hay en este repositorio una capa de interfaz o composición productiva que cierre esas brechas. Este documento registra el estado; no propone migraciones, rediseño ni tecnologías futuras.

---

## Trazabilidad principal

### Dominio y aplicación

- `internal/domain/resource_types.go`
- `internal/domain/resource_validation.go`
- `internal/domain/resource_canonical.go`
- `internal/domain/resource_catalog.go`
- `internal/domain/resource_catalog_validate.go`
- `internal/domain/resource_scope.go`
- `internal/domain/resource_repository_v2.go`
- `internal/domain/catalog_kind.go`
- `internal/domain/catalog_mutation.go`
- `internal/domain/catalog_admin_v2.go`
- `internal/domain/core_errors.go`
- `internal/app/recursos/service.go`
- `internal/app/catalogo/service.go`

### PostgreSQL

- `internal/postgres/resource_repository_crud.go`
- `internal/postgres/resource_repository_attributes.go`
- `internal/postgres/resource_repository_codec.go`
- `internal/postgres/resource_repository_search.go`
- `internal/postgres/catalog_loader.go`
- `internal/postgres/catalog_admin_repository.go`
- `internal/postgres/catalog_admin_kinds.go`
- `internal/postgres/catalog_admin_repository_v2.go`

### Contrato público y bridge

- `resourcecore/reader.go`
- `resourcecore/writer.go`
- `resourcecore/types.go`
- `resourcecore/queries.go`
- `resourcecore/write_types.go`
- `resourcecore/errors.go`
- `internal/bridge/resourcecore/adapter.go`
- `internal/core/errors.go`

### Migraciones

- `migrations/000002_resource_master.up.sql`
- `migrations/000003_catalog_admin.up.sql`
- `migrations/000004_unit_names.up.sql`
- `migrations/000005_resource_integrity.up.sql`
- `migrations/000006_supplier_master.up.sql` — verificada; pertenece a Supplier Master y no altera Resource Master
- `migrations/000007_resource_identity_v1.up.sql`
- `migrations/000008_resource_revisions.up.sql`

### Pruebas representativas

- `internal/domain/resource_*_test.go`
- `internal/app/recursos/*_test.go`
- `internal/app/catalogo/*_test.go`
- `internal/postgres/resource_repository_integration_test.go`
- `internal/postgres/resource_repository_search_integration_test.go`
- `internal/postgres/resource_lifecycle_integration_test.go`
- `internal/postgres/catalog_loader_integration_test.go`
- `internal/postgres/catalog_admin_repository_v2_integration_test.go`
- `resourcecore/*_test.go`
- `internal/bridge/resourcecore/adapter_test.go`

---

## 16. Diccionario de modelo campo por campo

Esta sección amplía el grafo conceptual con el contrato concreto. “Obligatorio” significa requerido por el modelo o por la validación para el uso normal; no implica que el tipo Go impida su valor cero.

### 16.1 Recurso, scope, comandos y paginación

| Tipo.campo | Tipo Go | Propósito y relación | Obligatoriedad / invariante | Lifecycle / persistencia | Evidencia |
|---|---|---|---|---|---|
| `Resource.ID` | `int64` | PK estable de `recursos` | positivo para update/lifecycle; cero antes de persistir | no cambia | `resource_types.go:164-180`; `service.go:110-113` |
| `Resource.ClassCode` | `string` | clase propietaria | no vacío; debe resolver activa en writes | puede cambiar por update | `resource_validation.go:15-22`; `resource_repository_crud.go:470-490` |
| `Resource.FamilyCode` | `string` | familia dentro de clase | no vacío; coherente con clase | puede cambiar por update | `resource_scope.go:9-15` |
| `Resource.TypeCode` | `string` | tipo dentro de familia/clase | obligatorio para recurso; coherente con familia | puede cambiar por update | `resource_validation.go:20-22` |
| `Resource.NaturalUnit` | `string` | unidad de gestión del recurso | política activa, permitida; no integra identidad | reemplazable | `resource_validation.go:23-26`; `ResourceUnitPolicy` |
| `Resource.Attributes` | `[]ResourceAttributeValue` | valores efectivos | códigos únicos; required/forbidden/relaciones/tipos válidos | reemplazo completo, no patch | `resource_validation.go:27-82`; `resource_repository_crud.go:499-508` |
| `Resource.IdentityKey` | `string` | clave natural durable `v1` | canonical, no vacía, única por clase | legacy update la cambia; CAS la conserva | `resource_validation.go:83-92`; migraciones 000002/000007 |
| `Resource.Revision` | `uint64` | token CAS | HEAD persistido >0; cero en lecturas ordinarias actuales | +1 por cambio CAS real | `resource_types.go:172-177`; migración 000008 |
| `Resource.Active` | `bool` | estado lógico | lectura histórica permitida | deactivate/reactivate | `Resource.Deactivate`/`Reactivate` |
| `Resource.canonical` | `bool` privado | prueba de construcción autoritativa | `ValidateForPersistence` exige `true` | no se persiste | `resource_types.go:179,235-240` |
| `ResourceScope.ClassCode` | `string` | raíz de narrowing | obligatorio para recursos | canonicalizado | `resource_scope.go:11-23` |
| `ResourceScope.FamilyCode` | `string` | narrowing class-owned | obligatorio | canonicalizado | `resource_scope.go:11-23` |
| `ResourceScope.TypeCode` | `string` | narrowing opcional en consultas de catálogo | obligatorio en recurso, vacío significa scope familia | canonicalizado | `resource_scope.go:7-15,35-47` |
| `CreateCommand.Scope` | `ResourceScope` | intención de scope | obligatorio | produce `Resource` nuevo | `resource_types.go:198-202` |
| `CreateCommand.NaturalUnit` | `string` | intención de unidad | obligatoria | validada por catálogo | ídem |
| `CreateCommand.Attributes` | `[]ResourceAttributeValue` | intención de valores | puede ser vacía sólo si el tipo no exige atributos | canonicalizada | ídem |
| `UpdateCommand.ID` | `int64` | target estable | `>0` | preservado | `resource_types.go:204-209` |
| `UpdateCommand.Scope/NaturalUnit/Attributes` | varios | estado de reemplazo | mismas reglas que create | reemplazo completo | `recursos.Service.Update*` |
| `ResourceSnapshot.*` | equivalentes a `Resource` | forma de hidratación desde persistencia | ID/scope/unidad completos e identidad prefijada `v1|` | `Revision` se copia sin validar | `resource_types.go:211-233` |
| `SearchCriteria.Text` | `string` | substring sobre identidad/familia | opcional | AND con el resto | `resource_types.go:245-260`; search SQL |
| `SearchCriteria.ClassCode/FamilyCode/TypeCode` | `string` | filtros exactos por código | opcionales | AND | ídem |
| `SearchCriteria.Filters` | `[]ResourceAttributeValue` | filtros tipados exactos | internos; no expuestos públicamente | un `EXISTS` por filtro | `resource_repository_search.go:84-90` |
| `SearchCriteria.LifecycleScope` | enum de 2 estados | activos o inactivos | sólo Active/Inactive | `ScopeAll` interno inexistente | `resource_types.go:40-45,267-277` |
| `SearchCriteria.Limit/Offset` | `int` | página | límite 0→50; máximo 50; negativos inválidos | no persistido | ídem |
| `ResourcePage.Criteria/Resources/HasPrevious/HasNext` | varios | página normalizada y señales | orden estable | sólo lectura | `resource_types.go:280-285` |
| `LifecycleResult.Resource` | `Resource` | estado resultante | completo cuando repositorio lo devuelve | público pierde `Changed` | `resource_types.go:47-50`; bridge lifecycle |
| `LifecycleResult.Changed` | `bool` | distingue transición de no-op | derivado | no expuesto en `resourcecore` | ídem |

### 16.2 Catálogo tipado de dominio

| Tipo | Campos exactos | Propósito / relaciones | Invariantes y lifecycle | Evidencia |
|---|---|---|---|---|
| `ResourceClass` | `Code, Name, Plural, Slug string`; `Aliases, Keywords []string`; `Order int`; `Active bool` | raíz; etiquetas, búsqueda y orden | code/slug únicos; textos no vacíos; inactive excluye nuevas selecciones | `resource_class.go:3-20` |
| `ResourceFamily` | `ClassCode, Code, Name string`; `Active bool` | hija de clase | `(ClassCode,Code)` único; familia activa no puede colgar de clase inactiva | `resource_catalog_validate.go:20-30` |
| `ResourceType` | `ClassCode, FamilyCode, Code, Name string`; `Active bool` | hijo de familia | familia/clase coherentes; tipo activo no puede colgar de familia inactiva | `resource_catalog_validate.go:32-40` |
| `PresentationField` | `ClassCode, FamilyCode, TypeCode, AttributeCode string`; `Position int`; `Active bool` | subset ordenado para descripción | atributo aplicable; posición única por tipo | `resource_types.go:52-65`; migración 000002:124-130 |
| `UnitDefinition` | `Code, Name, Symbol, Dimension string`; `Active bool` | unidad natural o de cantidad | code/symbol únicos; cuatro textos no vacíos | `resource_types.go:73-80`; migración 000004 |
| `ResourceUnitPolicy` | `ClassCode, FamilyCode, UnitCode string`; `Allowed, Suggested, Active bool` | unidad permitida por familia | `Suggested => Allowed`; write exige allowed+active | `resource_catalog_validate.go:56-62`; migración 000002:50-57 |
| `AttributeDefinition` | `Code, Name string`; `ValueType`; `Dimension string`; `DefaultIdentityParticipates, Active bool` | definición global de característica | code único; tipo dentro de seis enums | `resource_types.go:82-89` |
| `ResourceAttribute` | `ClassCode, FamilyCode, TypeCode, OptionSet string`; `Definition`; `Mode`; `IdentityParticipates bool`; `Rules []AttributeRule`; `Active bool` | binding por familia o tipo | scope único; `CONDITIONAL` sii reglas; definición/set activos en writes | `resource_catalog_validate.go:42-54` |
| `AttributeCondition` | `AttributeCode, Equals string` | antecedente de regla | referencia otro atributo y valor canónico | `resource_types.go:91-94` |
| `AttributeRule` | `When`; `Mode`; `IdentityParticipates, NotApplicable, Active bool` | semántica efectiva ordenada | regla inactiva bloquea nuevas escrituras; puede volver forbidden/N/A | `resource_types.go:96-102` |
| `ResourceOptionSet` | `Code, Name string`; `Active bool` | namespace de opciones | code natural; vacío se canonicaliza como `DEFAULT` | `resource_types.go:340-344` |
| `AttributeOption` | `OptionSet, AttributeCode, Code, Label string`; `Active bool` | valor controlado | debe pertenecer al set+definición; sólo activa en nuevas escrituras | `resource_types.go:104-114` |
| `AttributeOptionRelation` | `OptionSet, FromAttribute, FromOption, ToAttribute, ToOption string`; `Active bool` | restringe pares | mismo set, atributos distintos, opciones existentes | `resource_types.go:308-319` |
| `Quantity` | `Value decimal.Decimal`; `UnitCode string` | payload de cantidad | unidad no vacía, activa y dimensión compatible | `resource_types.go:116-119` |
| `ResourceAttributeValue` | `AttributeCode`; `Type`; `OptionCode`; `Integer *int64`; `Decimal *decimal.Decimal`; `Quantity *Quantity`; `Boolean *bool`; `Text` | unión tipada | exactamente un payload para `SET`; ninguno para N/A | `resource_types.go:121-130`; migración 000002:150-180 |
| `ResourceCatalog` | once slices: `Classes, Families, Types, PresentationFields, Units, UnitPolicies, Definitions, Attributes, Options, Relations, OptionSets` | snapshot completo | `Validate` agrega defectos con `errors.Join`; copia/publicación coherente | `resource_types.go:321-338`; `resource_catalog_validate.go:64-80` |

### 16.3 Modelo genérico de administración

| Tipo.campo | Propósito | Regla | Evidencia |
|---|---|---|---|
| `CatalogRef.Kind/Code/Label` | referencia por identidad natural; `Label` sólo presentación | dominio no usa ID DB para cross-ref | `catalog_record.go:5-19` |
| `CatalogValue.Text/Bool/Int/List/Ref` | unión según `FieldDescriptor.Kind` | el descriptor decide qué miembro tiene sentido | `catalog_record.go:21-31` |
| `CatalogRuleRecord.When/Mode/IdentityParticipates/NotApplicable/Active` | reglas ordenadas del agregado aplicabilidad | nil y slice vacía se preservan | `catalog_record.go:37-46` |
| `CatalogRecord.Kind/ID/Revision/Active/Values/Rules` | fila genérica de uno de 11 kinds | ID 0 pre-insert; revision 0 no persistido; mutable deep-copy | `catalog_record.go:48-60` |
| `CatalogFilter.Text/Parent/Status/Limit/Offset` | búsqueda administrativa | status desconocido falla cerrado a activos; parent interno no público | `catalog_record.go:107-126` |
| `CatalogDependency.Kind/Count/Blocking` | referencia hija observada | cualquier count no cero bloquea hard-delete en servicio, incluso si descriptor dice no bloqueante | `catalog_record.go:128-134`; `service.go:300-307` |
| `FieldDescriptor` | `Name,Label,Kind,Required,Immutable,Searchable,Guidance,RefKind,RefScopedBy,AllowCreate,EnumValues` | esquema fijo, no introspección SQL | `catalog_kind.go:57-70` |
| `CatalogKind` | `Code,Singular,Plural,Fields,IdentityFields,ParentKind,ParentField,SoftDelete,Children` | los 11 tienen lifecycle | `catalog_kind.go:80-97`; `TestCatalogKindRegistry_AllRegisteredKindsAreLifecycleCapable` |

## 17. Esquema PostgreSQL completo: HEAD frente a base viva

### 17.1 Delimitación

- **Base viva confirmada:** migración 4. Contiene 14 tablas de Resource Master más `schema_migrations`, total 15 tablas públicas observadas.
- **HEAD Resource Master:** las mismas 14, más `resource_integrity_identity_map` de migración 5: 15 tablas de negocio Resource Master. Migración 8 añade columnas, no tablas.
- **Migración 6:** `000006_supplier_master.up.sql` crea `suppliers`, `supplier_branches` y `supplier_contacts`; es otro bounded context y **no modifica Resource Master**.
- **HEAD total potencial:** además de las 15 tablas Resource Master existen las tres de Supplier Master y `schema_migrations`; este documento no las contabiliza como modelo Resource Master.

### 17.2 Tabla por tabla

| Tabla | Columnas en HEAD (`!` NOT NULL; `?` nullable) | Claves, checks, índices, cascadas | `active` / `created_at` y `updated_at` / `revision` | Base viva migración 4 |
|---|---|---|---|---|
| `resource_classes` | `id BIGSERIAL!`, `code TEXT!`, `name TEXT!`, `plural TEXT!`, `slug TEXT!`, `display_order INT!`, `active BOOL!`, `created_at!`, `updated_at!`, `aliases TEXT[]!`, `keywords TEXT[]!`, `revision BIGINT!` | PK id; UQ code/slug; checks no blank; índice implícito por UQ; trigger updated_at | sí / sí / HEAD sí | existe; sin revision |
| `resource_option_sets` | `code TEXT!`, `name TEXT!`, `active BOOL!`, `created_at!`, `updated_at!`, `revision BIGINT!` | PK code; no blank; FK desde opciones/relaciones/bindings; trigger | sí / sí / HEAD sí | existe; sin revision |
| `resource_families` | `id BIGSERIAL!`, `class_id BIGINT!`, `code/name/description TEXT!`, `active BOOL!`, `created_at`, `updated_at`, `revision` | PK; FK clase; UQ `(class_id,code)` y `(id,class_id)`; trigger | sí / sí / HEAD sí | existe; sin revision |
| `resource_types` | `id BIGSERIAL!`, `class_id/family_id BIGINT!`, `code/name TEXT!`, `active BOOL!`, `created_at`, `updated_at`, `revision` | FK compuesta `(family_id,class_id)`; UQ `(family_id,code)`, `(id,family_id)`; trigger | sí / sí / HEAD sí | existe; sin revision |
| `unit_definitions` | `id`, `code`, `name`, `symbol`, `dimension`, `active`, `created_at`, `updated_at`, `revision` | PK; UQ code/symbol; checks no blank, incluido `name` desde 000004; trigger | sí / sí / HEAD sí | existe; `name` presente; sin revision |
| `resource_unit_policies` | `family_id/unit_id BIGINT!`, `allowed/suggested/active BOOL!`, `created_at`, `updated_at`, `revision` | PK `(family_id,unit_id)`; FKs familia/unidad; check `NOT suggested OR allowed`; trigger | sí / sí / HEAD sí | existe; sin revision |
| `attribute_definitions` | `id`, `code/name/value_type`, `dimension?`, `default_identity_participates`, `active`, `created_at`, `updated_at`, `revision` | PK; UQ code; enum check de seis value types; trigger | sí / sí / HEAD sí | existe; sin revision |
| `attribute_options` | `option_set`, `attribute_definition_id`, `code`, `label`, `numeric_value?`, `unit_id?`, `active`, `display_order`, `created_at`, `updated_at`, `revision` | PK `(set,definition,code)`; FK set/definition/unit; check numeric y unit ambos nulos o ambos no nulos; UQ `(set,definition,display_order)`; trigger | sí / sí / HEAD sí | existe; sin revision |
| `attribute_option_relations` | `id`, `option_set`, ids/códigos from/to, `active`, `created_at`, `updated_at`, `revision` | PK; UQ de relación; FKs compuestas a opciones; check definiciones distintas; trigger | sí / sí / HEAD sí | existe; sin revision |
| `resource_attributes` | `id`, `class_id/family_id`, `type_id?`, `definition_id`, `option_set`, `mode`, `identity_participates`, `active`, `display_order`, `created_at`, `updated_at`, `revision` | FKs compuestas; índices UQ parciales por familia/tipo para definición y orden; UQ `(id,family_id,definition_id)`; las columnas inline de condición fueron eliminadas en 000003; trigger | sí / sí / HEAD sí | existe en forma post-000003; sin revision |
| `resource_attribute_rules` | `id`, `resource_attribute_id`, `display_order`, `when_definition_id`, `when_equals`, `mode`, `identity_participates`, `not_applicable`, `active`, `created_at`, `updated_at` | PK; FK parent `ON DELETE CASCADE`; FK definición; UQ `(parent,order)`; checks enum/no blank; trigger | sí / sí / **no** (revision del parent) | existe igual a HEAD |
| `resource_type_presentation_fields` | `type_id`, `attribute_definition_id`, `position`, `active`, `created_at`, `updated_at`, `revision` | PK `(type,attribute)`; UQ `(type,position)`; FKs; trigger | sí / sí / HEAD sí | existe; sin revision |
| `recursos` | `id`, `class_id/family_id/type_id/natural_unit_id`, `display_name`, `identity_key`, `active`, `created_at`, `updated_at`, `revision` | PK; UQ `(class_id,identity_key)`; FKs compuestas; UQ `(id,family_id)`; índice `class_id`; check `identity_key LIKE 'v1|%'` desde 000007 | sí / sí / HEAD sí | existe; sin revision y sin check v1, aunque datos lo cumplen |
| `resource_attribute_values` | `id`, `resource_id/family_id/resource_attribute_id/attribute_definition_id`, `option_set`, `value_state`, seis payloads nullable, `quantity_unit_id?` | UQ `(resource,resource_attribute)`; FK recurso `ON DELETE CASCADE`; FKs binding/opción/unidad; checks unión; trigger `validate_resource_attribute_value` | no / no / no | existe igual a HEAD |
| `resource_integrity_identity_map` | `resource_id`, `class_id`, `legacy_identity_key`, `v1_identity_key`, `mapped_at` | PK/FK recurso `ON DELETE CASCADE`; UQ `(class,legacy)` y `(class,v1)` | no / `mapped_at` / no | **ausente** |

### 17.3 Matriz de autoridad y enforcement

| Regla | Go | PostgreSQL | Ambos |
|---|:---:|:---:|:---:|
| scope clase/familia/tipo coherente | `ResourceCatalog.Validate`, `NewResource` | FKs compuestas | sí |
| nombres/códigos obligatorios | validación parcial/completa según tipo | `NOT NULL` + `CHECK btrim` | sí |
| binding único por scope | loader y resolución exigen uno | índices únicos parciales | sí |
| unión tipada de valor | canonicalización + codec | checks + trigger | sí |
| `NOT_APPLICABLE` sin payload | codec/dominio | checks | sí |
| option oficial | dominio | FK compuesta | sí |
| relación entre opciones | dominio | FKs + check definiciones distintas | sí |
| `suggested => allowed` | `validateUnitPolicies` no lo comprueba | check directo | PostgreSQL |
| hijos activos bajo padres activos | `ResourceCatalog.Validate` | sin constraint cross-table | Go |
| `CONDITIONAL` sii reglas | `validateAttributeRules` | sin constraint cross-table desde 000003 | Go |
| identidad v1 | `NewResource`/`HydrateResource` | check desde 000007 | ambos en HEAD; sólo Go/práctica en viva |
| unicidad de identidad por clase | dominio deriva; servicio clasifica | UQ global a activos/inactivos | ambos |
| revisión positiva | servicios rechazan expected=0 | check revision>0 desde 000008 | ambos en HEAD; ausente viva |
| incremento CAS | repositorios SQL autoritativos | `UPDATE revision=revision+1`; sin trigger | PostgreSQL mediante adaptador |
| hard-delete conservador de catálogo | servicio | FKs y `23503` como backstop | ambos |

## 18. Inventario completo de operaciones

### 18.1 Recursos internos

| Operación | Entrada | Salida | Validación / persistencia | Error relevante | Pública |
|---|---|---|---|---|---:|
| `Get` | class+identity | `Resource` | lectura histórica + hidratación efectiva | not found/integrity | sí |
| `Search` | `SearchCriteria` | slice | wrapper de página | invalid search/integrity | no directa |
| `SearchPage` | criteria | `ResourcePage` | normalize + repositorio paginado | invalid search | sí |
| `Describe` | `Resource` | string | snapshot actual, sin I/O | cadena vacía/fallback | sí por key |
| `Create` | `CreateCommand` | canonical `Resource` sin ID de repo | `NewResource` + tx create | duplicate/reference/integrity | sí |
| `Update` | `UpdateCommand` | canonical `Resource` | reemplazo legacy, recalcula identity | not found/duplicate | no |
| `Deactivate` | id | `LifecycleResult` | lock; idempotente | not found | no |
| `Delete` | id | error | alias de deactivate, nunca físico | idem | no |
| `Reactivate` | id | `LifecycleResult` | recomputa identidad y catálogo; lock | identity/reactivation/reference | no |
| `DeactivateRevision` | id+rev | `LifecycleResult` | CAS; incremento si cambia | conflict/not found | sí |
| `ReactivateRevision` | id+rev | `LifecycleResult` | prevalidación + CAS | conflict/identity/reactivation | sí |
| `UpdateRevision` | command+rev | `Resource` | CAS, parent+valores atómicos, identity preservada | conflict/reference/integrity | sí |

### 18.2 Catálogo interno

| Operación exportada de `catalogo.Service` | Semántica | Repositorio | Publicación |
|---|---|---|---|
| `Kinds` | devuelve 11 descriptores | ninguna | no |
| `List` | lista con `CatalogFilter` completo, incluido `Parent` | legacy read | no |
| `Get` | obtiene por kind+ID | legacy read | no |
| `Dependencies` | cuenta refs hijas | legacy probe | no |
| `ReferencedByResources` | detecta uso real | legacy probe | no |
| `Create` | valida candidato; usa V2 si está configurado, legacy si no | Insert | después de persistir |
| `Update` | reemplazo legacy + guard código inmutable | Update | después de persistir |
| `Deactivate` / `Reactivate` | lifecycle legacy | SetActive | después de persistir |
| `Delete` | hard-delete legacy conservador | Delete | después de persistir |
| `WithCatalogAdminRepositoryV2` | configura writer CAS | ninguna | no |
| `CatalogAdminRepositoryV2Configured` | consulta composición | ninguna | no |
| `ResetCatalogWriterAvailability` | libera latch tras reload/restart externo | ninguna | no |
| `CreateV2` | create coherente | V2 Insert | catálogo recargado por repo |
| `UpdateRevision` | replace CAS | V2 Update | resultado coherente |
| `DeactivateRevision` / `ReactivateRevision` | lifecycle CAS | V2 SetActive si no es no-op | resultado coherente |
| `HardDeleteRevision` | hard-delete CAS conservador | V2 Delete | resultado coherente |

Total: **18 métodos exportados** en `catalogo.Service`; el inventario agrupa pares lifecycle pero no los elimina del conteo.

### 18.3 Contrato público

| Operación | Request/query | Resultado | Bridge / pérdida conocida |
|---|---|---|---|
| `ActiveClasses` | ninguna | `[]CatalogRecord` | status activo fijo |
| `CatalogDescriptors` | ninguna | 11 descriptores | no expone `Immutable`, `Searchable`, `Guidance`, `AllowCreate`, `SoftDelete`, `Children` |
| `ListCatalog` | kind/scope/text/limit/offset | `CatalogPage` | no expone `Parent`; limit 0 anómalo |
| `GetCatalog` | kind+ID | record | revisión depende de lectura legacy |
| `SearchResources` | scope/text/class/family/type/page | `ResourcePage` | no filtros tipados; `ScopeAll` rechazado |
| `GetResource` | class+identity | resource | Get ordinario deja revision 0 |
| `DescribeResource` | class+identity | string | hace Get y luego Describe |
| `CreateCatalog` | actor/kind/active/values/rules | record | V2 sólo si service fue compuesto con repoV2 |
| `CreateResource` | actor/scope/unit/attrs | resource | create-confirm Get no transaccional; revision 0 |
| `UpdateCatalog` | actor/kind/id/rev/reemplazo | record | CAS V2 requerido |
| `UpdateResource` | actor/id/rev/reemplazo | resource | CAS preserva identity |
| `DeactivateCatalog` / `ReactivateCatalog` | actor/kind/id/rev | record | confirm-read posterior |
| `DeactivateResource` / `ReactivateResource` | actor/id/rev | resource | usa resultado completo; omite `Changed` |
| `HardDeleteCatalog` | actor/kind/id/rev | error | sin confirm-read; no existe equivalente de recurso |

Total público: **7 lecturas + 9 escrituras = 16 operaciones**.

## 19. Reglas ejecutables e invariantes

La tabla siguiente sustituye la matriz agrupada de §15.2 como fuente detallada. “Tx” indica dónde la regla participa en atomicidad; “—” significa evaluación pura o de frontera.

| ID | Condición válida | Violación | Símbolo Go | Refuerzo PostgreSQL | Tx | Error observable |
|---|---|---|---|---|---|---|
| RM-INV-001 | familia referencia clase existente | clase ausente | `validateFamilies` | FK `resource_families.class_id` | catálogo write | `INVALID_REFERENCE` |
| RM-INV-002 | tipo pertenece a familia+clase coherentes | combinación ajena | `validateTypes`, `hasTypeIn` | FK compuesta type→family | catálogo/resource write | `INVALID_REFERENCE` |
| RM-INV-003 | código de familia único dentro de clase | duplicado mismo scope | `validateFamilies` | UQ `(class_id,code)` | catálogo write | `VALIDATION`/`DUPLICATE` |
| RM-INV-004 | código de tipo único dentro de familia | duplicado | no comprobado por `ResourceCatalog.Validate` | UQ `(family_id,code)` | catálogo write | `DUPLICATE` desde repositorio |
| RM-INV-005 | hijo activo sólo bajo parent activo | familia/tipo activo bajo parent inactivo | `validateActiveHierarchy` | sin constraint cross-table | catálogo write | `INVALID_CATALOG` |
| RM-INV-006 | unidad sugerida también permitida | suggested=true, allowed=false | no comprobado por `validateUnitPolicies` | CHECK `NOT suggested OR allowed` | catálogo write | error SQL clasificado por repositorio |
| RM-INV-007 | definición/value usa un payload acorde | payload/tipo incompatible | `canonicalValue`, `encodeValue` | checks + trigger | resource write | `VALIDATION`/`INVALID_REFERENCE` |
| RM-INV-008 | exactamente un binding efectivo | cero o más de uno | `resourceAttributes`, `resolveAttributeTarget` | índices UQ parciales | resource tx/search | `INTEGRITY` |
| RM-INV-009 | `ModeConditional` iff `len(Rules)>0` | condicional sin reglas o inversa | `validateAttributeRules` | no | catálogo write | `INVALID_CATALOG` |
| RM-INV-010 | primera regla aplicable determina modo/identidad/N/A | ninguna regla para conditional | `ResourceAttribute.effective`, `NewResource` | rules ordenadas por display_order | — | `VALIDATION` |
| RM-INV-011 | opción pertenece a set+característica | código de otro set/definición | `canonicalValue`, `OptionsFor` | FK compuesta value→option | resource tx | `VALIDATION`/`INVALID_REFERENCE` |
| RM-INV-012 | relación conecta opciones válidas del mismo set | opción/set/atributo inconsistente | `validateRelations` | dos FKs compuestas + check | resource/catalog tx | `VALIDATION`/`INVALID_REFERENCE` |
| RM-INV-013 | presentación usa atributo aplicable y posición única | atributo ajeno/posición repetida | pruebas de consistencia de fixture; `Describe` consume sin validar | PK + UQ `(type,position)`; FK a tipo/definición | catálogo write | `DUPLICATE`/`INVALID_REFERENCE` desde repositorio |
| RM-INV-014 | sólo `Resource` canonical persiste | struct forjado | `ValidateForPersistence` | — | antes de BEGIN | `INVALID_REFERENCE` |
| RM-INV-015 | required presente; forbidden no `SET` | ausencia/presencia inválida | `NewResource` | cardinalidad/union, no required cross-table | — | `VALIDATION` |
| RM-INV-016 | N/A sin payload | sentinel con otro miembro | `encodeValue`, `hasPayload` | checks `num_nonnulls=0` | resource tx | `INVALID_REFERENCE` |
| RM-INV-017 | cantidad tiene unidad activa y compatible | nil/inactiva/dimensión inválida | `canonicalValue`, `validateActiveChain`, `resolveAttributeTarget` | FK unidad | resource tx | `VALIDATION`/`INTEGRITY` |
| RM-INV-018 | identidad incluye scope y partes identitarias ordenadas | identidad distinta | `NewResource`, `identityComponent` | check v1 desde 000007 | — | `VALIDATION` al hidratar/revalidar |
| RM-INV-019 | componente usa longitud UTF-8 | colisión por delimitador | `identityComponent` | `resource_identity_component` en 000005 | migración 000005 | migración aborta |
| RM-INV-020 | unidad natural no integra identidad | identidad cambia sólo por unidad | `NewResource` no agrega unidad | — | — | expectativa probada |
| RM-INV-021 | identidad única por clase, incluso inactiva | segunda fila igual | derivación + clasificación | UQ `(class_id,identity_key)` no parcial | resource tx | `DUPLICATE` |
| RM-INV-022 | lectura histórica tolera catálogo inactivo | hidratación exige active | `HydrateResource` no consulta actividad | — | — | lectura válida |
| RM-INV-023 | reactivación valida catálogo e identidad actual | referencias inactivas/identity divergente | `reactivationCandidate`, `setLifecycle*` | UPDATE con joins activos | lifecycle tx | `REACTIVATION_IMPOSSIBLE`/`IDENTITY_CONFLICT` |
| RM-INV-024 | lifecycle al estado actual es no-op | incremento o write innecesario | `Deactivate`/`Reactivate`, `setLifecycle*` | lock + branch | lifecycle tx | éxito, `Changed=false` |
| RM-INV-025 | transición CAS real incrementa una revisión | no incrementa o incrementa más | `setLifecycleRevision` | `revision=revision+1` | lifecycle tx | resultado rev+1 |
| RM-INV-026 | no-op CAS conserva revisión | revisión cambia | `setLifecycleRevision`; catálogo pre-check | sin UPDATE | lifecycle tx/ninguna | éxito |
| RM-INV-027 | revisión obsoleta no escribe | stale rev modifica | `ResourceRepositoryV2`, V2 catalog | lock + comparación/predicado CAS | write tx | `CONFLICT` |
| RM-INV-028 | update reemplaza todos los valores atómicamente | mezcla parcial | `Update`, `UpdateRevision` | DELETE+INSERT+count bajo misma tx | resource tx | rollback/error |
| RM-INV-029 | un único valor efectivo por atributo | fallback+specific duplican | search CTE `scope_count`, resolver | UQ no impide overlap broad/specific | read/resource tx | `INTEGRITY` |
| RM-INV-030 | búsqueda ordenada y acotada | página inestable/excesiva | `Normalize`, `SearchPage` | `ORDER BY identity_key,id LIMIT+1` | read | `INVALID_ARGUMENT` o página |
| RM-INV-031 | mutación catálogo se valida antes de persistir | candidato inválido llega al repo | `ApplyCatalogMutation`, `Validate` | constraints de backstop | service mutex + repo tx | `INVALID_CATALOG`/dominio |
| RM-INV-032 | publicar sólo después de persistencia confirmada | snapshot adelanta DB | `publish`, `publishCoherent` | V2 recarga en tx | service mutex + repo tx | no publicación en error |
| RM-INV-033 | código referenciado y dependencias protegen write/delete | rename/delete en uso | `Update`, `buildV2DeleteCandidate` | FK / SQL omite SET code | catálogo tx | `IMMUTABLE_CODE`/`IN_USE` |
| RM-INV-034 | error público no expone causa | `Unwrap` o mensaje SQL | `core.Map`, `resourcecore.Error` | — | — | 15 códigos seguros |
| RM-INV-035 | colecciones públicas son copias | caller muta estado interno | clones Reader/Writer/bridge | — | — | aislamiento |
| RM-INV-036 | hard-delete público sólo para catálogo | recurso se borra físicamente vía Core | superficie `WriteCapabilities` | DB concede DELETE pero no caso de uso | catálogo tx | no existe operación recurso |
| RM-INV-037 | update legacy persiste nueva identidad canónica | identity vieja queda | `Service.Update`; repo `Update` | SET `identity_key=$6` | resource tx | duplicate/reference |
| RM-INV-038 | update CAS conserva identity almacenada | identity se regenera | `UpdateRevision` | identity ausente del SET | resource tx | comportamiento probado |
| RM-INV-039 | write CAS público exige expected revision >0 | token cero llega a app | validadores `Writer`; servicios | defensa en repositorios | antes de tx | `INVALID_ARGUMENT` |

### 19.1 Expectativas incumplidas, no invariantes ejecutables

Estas cinco filas estaban mezcladas con invariantes en §15.1. No son reglas que el sistema haga cumplir; son discrepancias observables.

| ID histórico | Expectativa | Estado factual | Evidencia |
|---|---|---|---|
| RM-INV-040 | una lectura ordinaria aporta revisión para iniciar CAS | **incumplida**: Get/Search/create-confirm producen 0 | `Get`, `SearchPage`, bridge `CreateResource` |
| RM-INV-041 | esquema ejecutor contiene columnas consultadas | **incumplida en DB viva**: migración 4 vs 8 | introspección viva + 000008 |
| RM-INV-042 | `ScopeAll` público de recursos representa ambos estados | **incumplida**: Reader lo valida como valor conocido, bridge lo rechaza | `reader.go:147-151`; `adapter.go:270-283` |
| RM-INV-043 | límite cero de catálogo tiene default coherente | **incumplida**: pide una fila, `HasNext=false` | `adapter.go:121-133,368-388` |
| RM-INV-044 | un write que devuelve error no dejó commit exitoso | **incumplida** al cambiar clase en CAS: recarga usa clase anterior | `resource_repository_crud.go:453-459,511-525` |

### 19.2 Ficha normalizada de invariantes ejecutables

| ID | Descripción | Origen | Implementación | Persistencia implicada | Error si se viola | Prueba existente |
|---|---|---|---|---|---|---|
| RM-INV-001 | familia sólo bajo clase existente | dominio + integridad referencial | `validateFamilies` | FK en catálogo write | `INVALID_REFERENCE` | `TestResourceCatalog_Validate_DefectCases` |
| RM-INV-002 | tipo conserva clase+familia coherentes | dominio + SQL | `validateTypes` | FK compuesta | `INVALID_REFERENCE` | `TestResourceCatalog_Validate_DefectCases` |
| RM-INV-003 | familia única dentro de clase | dominio + SQL | `validateFamilies` | UQ catalog tx | `VALIDATION`/`DUPLICATE` | test family-code scoped |
| RM-INV-004 | tipo único dentro de familia | PostgreSQL | UQ handler | UQ catalog tx | `DUPLICATE` | catalog CAS integration |
| RM-INV-005 | hijo activo no cuelga de parent inactivo | dominio | `validateActiveHierarchy` | candidato previo a write | `INVALID_CATALOG` | tests active hierarchy |
| RM-INV-006 | unidad sugerida implica permitida | PostgreSQL | CHECK | catalog tx | error clasificado | catalog integration |
| RM-INV-007 | payload coincide con value type | dominio + SQL | canonicalizer/codec/trigger | resource tx | `VALIDATION`/`INVALID_REFERENCE` | codec + resource integration |
| RM-INV-008 | un binding efectivo por atributo | dominio + SQL | resolver/scope count | índices + resource tx | `INTEGRITY` | cardinality integration |
| RM-INV-009 | conditional equivale a reglas presentes | dominio | `validateAttributeRules` | candidato catálogo | `INVALID_CATALOG` | dos tests conditional |
| RM-INV-010 | regla aplicable fija semántica efectiva | dominio | `effective` | reglas ordenadas cargadas | `VALIDATION` | conditional fixture test |
| RM-INV-011 | opción pertenece a set+característica | dominio + SQL | `canonicalValue` | FK value→option | `VALIDATION`/`INVALID_REFERENCE` | official-code/option-set tests |
| RM-INV-012 | pares de opciones respetan relación | dominio + SQL | `validateRelations` | relation FKs + resource tx | `VALIDATION` | diameter narrowing tests |
| RM-INV-013 | presentación referencia tipo/definición y posición única | PostgreSQL + fixture | `Describe` consume configuración | PK/UQ/FKs | `DUPLICATE`/`INVALID_REFERENCE` | presentation fixture tests |
| RM-INV-014 | sólo recurso canonical persiste | dominio | `ValidateForPersistence` | antes de BEGIN | `INVALID_REFERENCE` | canonical writes tests |
| RM-INV-015 | required presente y forbidden sin SET | dominio | `NewResource` | pre-persist | `VALIDATION` | conductor validation test |
| RM-INV-016 | N/A no lleva payload | dominio + SQL | `encodeValue` | checks en value tx | `INVALID_REFERENCE` | codec N/A test |
| RM-INV-017 | quantity lleva unidad válida/activa/compatible | dominio + SQL | canonical/active/target resolvers | FK + resource tx | `VALIDATION`/`INTEGRITY` | unit/value tests |
| RM-INV-018 | identidad v1 incluye scope y partes ordenadas | dominio + SQL | `NewResource` | check v1 HEAD | `VALIDATION` | canonical identity test |
| RM-INV-019 | componentes usan longitud UTF-8 | dominio + migración | `identityComponent` | migration audit tx | migración aborta | delimiter collision test |
| RM-INV-020 | unidad natural queda fuera de identidad | dominio | `NewResource` | — | identidad estable | natural-unit tests |
| RM-INV-021 | identidad única por clase también inactiva | SQL + dominio | derivación/mapping | UQ resource tx | `DUPLICATE` | duplicate + integration |
| RM-INV-022 | histórico se lee con catálogo inactivo | dominio | `HydrateResource` | read | lectura no falla | rehydrate history test |
| RM-INV-023 | reactivación revalida autoridad e identidad | aplicación + SQL | `reactivationCandidate` | lifecycle tx | `REACTIVATION_IMPOSSIBLE`/`IDENTITY_CONFLICT` | reactivation tests |
| RM-INV-024 | lifecycle al mismo estado es no-op | dominio + repositorio | lifecycle methods | lock/commit o pre-return | éxito sin cambio | lifecycle idempotence tests |
| RM-INV-025 | transición CAS real incrementa una vez | repositorio SQL | `setLifecycleRevision` | row-lock tx | conflict/fallo de resultado | lifecycle integration |
| RM-INV-026 | no-op CAS conserva revisión | aplicación/repositorio | branches no-op | sin UPDATE | éxito | lifecycle no-op tests |
| RM-INV-027 | stale revision no escribe | repositorio SQL | lock + compare/CAS | write tx | `CONFLICT` | concurrent revision test |
| RM-INV-028 | update reemplaza atributos atómicamente | repositorio SQL | DELETE+INSERT+count | resource tx | rollback/error | atomic CAS test |
| RM-INV-029 | búsqueda rechaza dos winners efectivos | repositorio SQL | CTE `scope_count` | read query | `INTEGRITY` | search/cardinality integration |
| RM-INV-030 | página es acotada y estable | dominio + SQL | Normalize/order/limit+1 | read query | `INVALID_ARGUMENT` | pagination + search integration |
| RM-INV-031 | candidato catálogo se valida antes del write | aplicación | mutation + `Validate` | mutex, luego repo | dominio/`INVALID_CATALOG` | create rejection test |
| RM-INV-032 | publicación sigue a commit confirmado | aplicación + repo V2 | `publish*` | repo tx + mutex | no publicación | coherence tests |
| RM-INV-033 | refs protegen rename/delete | aplicación + SQL | immutable/dependency guards | probes + FK tx | `IMMUTABLE_CODE`/`IN_USE` | guard + race tests |
| RM-INV-034 | error público es opaco | core/contrato | `core.Map`, public `Error` | — | código seguro | public errors/category tests |
| RM-INV-035 | mutable público cruza como copia | contrato/bridge | clone functions | — | aislamiento | Reader/Writer copy tests |
| RM-INV-036 | hard-delete de recurso no es capacidad Core | contrato | ausencia de método | DB fuera del caso de uso | operación inexistente | no-ungraduated-method test |
| RM-INV-037 | update legacy escribe identidad nueva | aplicación + SQL | legacy Update | resource tx SET identity | duplicate/reference | canonical update tests |
| RM-INV-038 | update CAS conserva identidad almacenada | repositorio SQL | identity fuera del SET | resource tx | conducta establecida | atomic CAS test |
| RM-INV-039 | CAS requiere expected revision no cero | contrato + aplicación + repo | validadores/guards | antes de tx | `INVALID_ARGUMENT` | Writer/service shape tests |

## 20. Lifecycle exacto

### 20.1 Recursos

| Estado origen | Operación | Estado destino | Permitido | Condiciones |
|---|---|---|---|---|
| activo | deactivate legacy | inactivo | sí | ID existente; lock `FOR UPDATE`; `Changed=true` |
| inactivo | deactivate legacy | inactivo | sí, no-op | ID existente; commit sin UPDATE; `Changed=false` |
| inactivo | reactivate legacy | activo | sí | identidad esperada coincide; clase/familia/tipo/unidad/política activos; sin conflicto |
| activo | reactivate legacy | activo | sí, no-op | la aplicación retorna antes o repo confirma sin UPDATE |
| activo | deactivate CAS | inactivo | sí | expected revision coincide; rev +1 |
| inactivo | deactivate CAS | inactivo | sí, no-op | expected revision se verifica en repositorio; rev igual |
| inactivo | reactivate CAS | activo | sí | prevalidación de catálogo e identidad; expected revision coincide; rev +1 |
| activo | reactivate CAS | activo | sí, no-op | **la aplicación retorna antes del CAS**; una revisión obsoleta puede no ser detectada |
| cualquiera | update legacy | conserva `active` almacenado | sí | scope/valores válidos; reemplazo total; identidad se recalcula |
| cualquiera | update CAS | conserva `active` almacenado | sí | expected revision; reemplazo total; rev +1; identidad almacenada no cambia |
| cualquiera | hard-delete Core | — | no encontrado | `Delete` es alias de deactivate |

### 20.2 Catálogo

| Estado origen | Operación | Estado destino | Permitido | Condiciones |
|---|---|---|---|---|
| inexistente | create | active según request | sí | kind registrado, candidato y catálogo válidos; revision 1 en HEAD |
| activo | update legacy/CAS | activo o valor `Active` del replacement | sí | guard de código si referenciado; CAS en V2 |
| inactivo | update legacy/CAS | inactivo o valor replacement | sí | mismas condiciones |
| activo | deactivate | inactivo | sí | candidato no deja hijos activos inválidos; CAS si V2 |
| inactivo | deactivate | inactivo | sí, no-op | V2 service retorna antes del repo; expected revision obsoleta puede quedar sin detectar |
| inactivo | reactivate | activo | sí | catálogo completo válido; CAS si V2 |
| activo | reactivate | activo | sí, no-op | mismo bypass de comprobación CAS en service V2 |
| activo | hard-delete | — | no | `ErrInvalidLifecycle` |
| inactivo referenciado | hard-delete | — | no | cualquier dependencia no cero o referencia de recurso -> `IN_USE` |
| inactivo sin referencias | hard-delete | inexistente | sí | expected revision en V2; FK como backstop; resultado sin Record |

## 21. Búsqueda y paginación

| Superficie | Función/query | Filtros | SQL / estrategia | Índice/plan confirmado | Límite |
|---|---|---|---|---|---|
| recursos internos | `resourceRepository.SearchPage` | lifecycle, clase, familia, tipo, texto, `Filters` tipados; todos AND | CTE de valores efectivos; query parent `limit+1`; segunda query set-based para atributos | índice UQ `(class_id,identity_key)` existe; plan vivo pequeño eligió seq scan; `ILIKE '%x%'` no mostró índice | 0→50; 1..50; offset>=0 |
| recursos público | `Adapter.SearchResources` | scope, texto, clase, familia, tipo | mapea a criteria; rechaza ALL | igual al interno | máximo 50 por Normalize |
| catálogo interno | `CatalogAdminRepository.List` | kind, status, texto, Parent, limit, offset | handler fijo por kind; identificadores sólo desde registro interno | índices PK/UQ/FK por tabla; no se midió a escala | repositorio recibe enteros |
| catálogo público | `Adapter.ListCatalog` | kind, scope, texto, limit, offset | solicita `limit+1`; no Parent | para clases activas, plan vivo seq scan+sort | limit 0 devuelve hasta 1 y `HasNext=false` |

Detalles confirmados de recursos:

1. lifecycle produce `r.active` o `NOT r.active`; no existe branch ALL;
2. texto usa un mismo parámetro `'%texto%'` contra `identity_key`, `family.code` y `family.name`;
3. cada filtro tipado se codifica mediante `effectiveValueFilter` y `EXISTS`;
4. `scope_count > 1` marca corrupción por aplicabilidad ambigua;
5. orden `identity_key ASC, id ASC` estabiliza boundaries;
6. `HasNext` surge de la fila adicional y `HasPrevious` de `offset>0`;
7. la integración prueba páginas 1/10/50 y dos consultas, no rendimiento productivo.

## 22. Concurrencia, aislamiento y atomicidad

### 22.1 Carrera A/B desde revisión 10

| Paso | Writer A | Writer B |
|---:|---|---|
| 1 | recibe recurso revisión 10 | recibe el mismo recurso revisión 10 |
| 2 | abre tx (aislamiento pgx por defecto: PostgreSQL Read Committed) | abre tx equivalente |
| 3 | `SELECT ... FOR UPDATE`; obtiene lock, lee 10 | espera el lock |
| 4 | valida `10==10`; ejecuta parent/valores o lifecycle; revision→11 | — |
| 5 | commit y libera lock | obtiene lock y lee 11 |
| 6 | éxito con revisión 11 | `11!=10` -> `ErrResourceRevisionConflict` -> público `CONFLICT` |

**Retries:** no se encontró retry automático para conflicto CAS, deadlock o serialización. El consumidor debe releer y decidir una nueva intención. Los repositorios no usan `Serializable`; el lock de fila más el token esperado es la protección explícita.

### 22.2 Fronteras transaccionales

| Flujo | Aislamiento / lock | Unidad atómica | Después del commit |
|---|---|---|---|
| create recurso | Read Committed, sin pre-lock | INSERT parent + N valores + conteo | devuelve sin ID; bridge hace Get aparte |
| update recurso legacy | Read Committed | resolve refs + UPDATE parent/identity + DELETE/INSERT valores + conteo | devuelve candidato de app |
| lifecycle legacy | Read Committed + `FOR UPDATE` | estado y guards | Get fuera de tx |
| update recurso CAS | Read Committed + `FOR UPDATE` | revisión+parent+DELETE/INSERT valores+conteo | `getWithRevision` fuera de tx; anomalía si cambia clase |
| lifecycle recurso CAS | Read Committed + `FOR UPDATE` | guard identidad/revisión + transición | `getWithRevision` fuera de tx |
| carga catálogo | `RepeatableRead`, `ReadOnly` | 12 queries físicas para 11 kinds; aplicabilidad usa queries separadas para bindings y rules | valida/clasifica catálogo |
| catálogo V2 | tx del repo, CAS/insert/delete + recarga completa+Validate | row write + snapshot coherente | service publica sólo `CatalogWriteResult.Catalog` |
| catálogo legacy | mutex de proceso + transacción/statement del repo | persistencia de fila; candidato se validó antes | service publica candidato local |

**Atomicidad confirmada:** `TestResourceRepositoryUpdateRevisionAtomicCAS` fuerza fallas de atributos y verifica rollback de parent y valores. **Límite:** los confirm-read y reload post-commit no forman parte de la transacción; por ello pueden fallar después de una mutación confirmada.

## 23. Catálogo de errores

| Código/Tipo | Significado | Operaciones | Público |
|---|---|---|---:|
| `INVALID_ARGUMENT` / `core.ErrInvalidArgument` | forma, ID, actor, scope o expected revision inválidos | Readers/Writers; servicios | sí |
| `NOT_FOUND` / `ErrResourceNotFound`, `ErrCatalogRecordNotFound` | target ausente | get/update/lifecycle/delete | sí |
| `DUPLICATE` / `ErrDuplicateResource`, `ErrCatalogDuplicate` | identidad o clave única ya existe | create/update/reactivate/catalog writes | sí |
| `INVALID_REFERENCE` / `ErrResourceReference`, `ErrCatalogReference` | clase/familia/tipo/unidad/opción/ref inexistente o inactiva | create/update/reactivate/catalog write | sí |
| `VALIDATION` / `ErrResourceValidation` | regla de negocio o estructura de request inválida | `NewResource`, mutación catálogo | sí |
| `INTEGRITY` / `ErrResourceIntegrity` | cardinalidad/persistencia incoherente | hydration, codec, resource repository | sí |
| `IDENTITY_CONFLICT` / `ErrIdentityConflict` | identidad durable difiere de canonical | reactivación | sí |
| `INVALID_LIFECYCLE` / `ErrInvalidLifecycle` | transición no idempotente prohibida | hard-delete activo | sí |
| `REACTIVATION_IMPOSSIBLE` / `ErrReactivationImpossible` | retenido no satisface catálogo actual | reactivate resource | sí |
| `INVALID_CATALOG` / `ErrInvalidCatalog` | snapshot candidato/cargado incoherente | catalog create/update/lifecycle/delete | sí |
| `IN_USE` / `ErrCatalogInUse` | dependencias o recursos impiden delete | hard-delete catálogo | sí |
| `IMMUTABLE_CODE` / `ErrCodeImmutable` | código referenciado no puede cambiar | update catálogo | sí |
| `CONFLICT` / `ErrRevisionConflict` | expected revision obsoleta | writes CAS | sí |
| `UNAVAILABLE` / `core.ErrUnavailable` | repo V2 no configurado, latch, cancel/deadline | catálogo V2 y contexto | sí |
| `INTERNAL` / fallback | fallo no clasificado | cualquier operación | sí |

**Precedencia:** revision → identity → reactivation → invalid catalog → lifecycle → immutable → in-use → integrity → validation → duplicate → reference → not-found → invalid-argument → unavailable → internal (`internal/core/errors.go:69-102`). El error neutral y `resourcecore.Error` no implementan `Unwrap`; la causa técnica no cruza la frontera.

**SQLSTATE de recursos:** `23505` es duplicate salvo UQ de cardinalidad de valores, que es integrity; `23503` y `23514` son reference. Catálogo posee clasificación específica adicional en sus handlers. No se parsean textos en `core.Map`.

## 24. DTOs, interfaces y mappings del bridge

### 24.1 Interfaces

| Interfaz | Métodos | Implementación comprobada |
|---|---|---|
| `domain.ResourceRepository` | Create, Get, Search, Update, SetActive, Deactivate, Reactivate | `resourceRepository`; constructor retorna interfaz |
| `domain.ResourcePageRepository` | SearchPage | type assertion en servicio |
| `domain.ResourceRepositoryV2` | DeactivateRevision, ReactivateRevision, UpdateRevision | `resourceRepository` |
| `domain.CatalogAdminRepository` | List, Get, Insert, Update, SetActive, Delete, Dependents, ReferencedByResources | PostgreSQL legacy |
| `domain.CatalogAdminRepositoryV2` | Insert, Update, SetActive, Delete | PostgreSQL V2; opcional en service |
| `resourcecore.ReadCapabilities` | 7 lecturas | assertion compile-time sobre bridge |
| `resourcecore.WriteCapabilities` | 9 escrituras | assertion compile-time sobre bridge |

### 24.2 Mapeo campo por campo

| Origen | Destino | Mapeo completo / transformación | Omisión o pérdida |
|---|---|---|---|
| `domain.Resource` | `resourcecore.Resource` | `ID→ID`, `IdentityKey→IdentityV1`, tres códigos→`Scope`, `NaturalUnit`, `Active`, `Revision`, atributos | `canonical` privado no se expone, correctamente |
| `domain.ResourceAttributeValue` | `resourcecore.AttributeValue` | `AttributeCode→Code`; tipo a `Value.Kind`; payload a Text/Bool; quantity duplica unit en `Value.UnitCode` y `AttributeValue.UnitCode` | punteros nil se representan como zero payload del kind |
| `resourcecore.AttributeValue` | dominio | siete kinds: controlled option, integer, decimal, quantity, bool, N/A, text | code/enum/reference/string-list no aplican y se rechazan |
| `domain.CatalogRecord` | público | Kind, ID, Revision, Active, Values y Rules completos | `CatalogRef.Label` no se expone; referencia pública lleva ID 0 y natural Code |
| `CatalogRuleRecord` | `ApplicabilityRule` | When.AttributeCode, Equals como TEXT, Mode, IdentityParticipates, NotApplicable, Active | ninguna de los seis campos |
| `domain.CatalogKind` | `CatalogDescriptor` | Code, labels, fields, IdentityFields, ParentKind/Field | no expone `SoftDelete`, `Children` |
| `domain.FieldDescriptor` | público | Name, Label, Kind, Required, RefKind, RefScopedBy, EnumValues | no expone `Immutable`, `Searchable`, `Guidance`, `AllowCreate` |
| `CatalogWriteRequest` | `CatalogRecord` | Kind separado, Active, Values según descriptor, Rules; Actor→context | Actor no persiste; create no envía revision |
| `CatalogUpdateRequest` | `CatalogRecord` + arg | ID, Active, Values, Rules; ExpectedRevision arg; Actor→context | request `Kind` se aplica fuera del record |
| `ResourceWriteRequest` | `CreateCommand` | Scope, NaturalUnit, todos los atributos; Actor→context | resultado de create exige confirm-read |
| `ResourceUpdateRequest` | `UpdateCommand` + arg | ID, Scope, NaturalUnit, atributos; ExpectedRevision arg | ninguna pérdida de fields de comando |
| `CatalogLifecycleRequest` | args service | Kind, ID, ExpectedRevision; Actor→context | no contenido por diseño |
| `ResourceLifecycleRequest` | args service | ID, ExpectedRevision; Actor→context | `LifecycleResult.Changed` no retorna público |
| `CatalogQuery` | `CatalogFilter` | Kind separado; Scope→Status, Text, Offset, `Limit+1` | `Parent` no existe público |
| `ResourceQuery` | `SearchCriteria` | Scope excepto ALL, Text, Class, Family, Type, Limit, Offset | `Filters` no existe público |

### 24.3 Validación de frontera frente a negocio

- `Reader` valida kind, lifecycle scope y claves mínimas; no valida existencia.
- `Writer` valida actor, IDs/revisión, scope/unidad y forma del union; no decide opciones, identidad, lifecycle o duplicados.
- El bridge convierte tipos y adjunta actor; dominio/aplicación siguen siendo autoridad semántica.
- `NewAdapter` no comprueba nil pese a su comentario; dereferencia `catalog.Kinds()` inmediatamente, por lo que nil no es una construcción válida en la práctica.

## 25. Los once kinds administrables

| Kind | Descriptor / identidad natural | Storage | Relaciones y dependencias | Lifecycle | ID genérico | Filas vivas |
|---|---|---|---|---|---|---:|
| `CLASE` | code, name, plural, slug, order, aliases, keywords / `code` | `resource_classes` | parent de familia; recursos también refieren clase | soft + hard guarded | BIGSERIAL | 3 |
| `FAMILIA` | class, code, name / `class+code` | `resource_families` | hija clase; parent de tipo, aplicabilidad y política; recursos | soft + hard guarded | BIGSERIAL | 2 |
| `TIPO` | class, family, code, name / triple | `resource_types` | hija familia; parent de aplicabilidad y presentación; recursos | soft + hard guarded | BIGSERIAL | 2 |
| `CARACTERISTICA` | code, name, valueType, dimension, defaultIdentity / `code` | `attribute_definitions` | opciones, bindings, rules, presentación, valores | soft + hard guarded | BIGSERIAL | 8 |
| `CONJUNTO_OPCIONES` | code, name / `code` | `resource_option_sets` | opciones, relaciones y bindings | soft + hard guarded | `hashtextextended(code)` | 1 |
| `OPCION` | set, characteristic, code, label / triple | `attribute_options` (PK compuesta) | relaciones y valores de recursos | soft + hard guarded | hash de clave natural | 54 |
| `RELACION_OPCIONES` | set, from characteristic/option, to characteristic/option / set+fromOption+toOption | `attribute_option_relations` | dos refs a opciones | soft + hard guarded | BIGSERIAL persistido | 9 |
| `UNIDAD` | code, name, symbol, dimension / `code` | `unit_definitions` | políticas, recursos naturales, cantidades | soft + hard guarded | BIGSERIAL | 2 |
| `POLITICA_UNIDAD` | class, family, unit, allowed, suggested / triple | `resource_unit_policies` (PK compuesta) | hija familia/unidad; recursos dependen semánticamente | soft + hard guarded | hash de clave natural | 2 |
| `APLICABILIDAD` | class, family, type?, characteristic, set?, mode, identity; Rules / scope+characteristic | `resource_attributes` + `resource_attribute_rules` | hija familia/tipo/def/set; parent de reglas y valores | soft + hard guarded; rules comparten revision | BIGSERIAL parent | 8 bindings + 2 rules |
| `PRESENTACION` | class, family, type, characteristic, position / scope+characteristic | `resource_type_presentation_fields` (PK compuesta) | hija tipo/definición | soft + hard guarded | hash de clave natural | 5 |

**Hardcoded frente a DB:** el registro, labels, campos, enums, identidad y grafo de dependencias están hardcoded en `catalog_kind.go`; las filas y estados runtime provienen de PostgreSQL. `SeedResourceCatalog` es fixture, no source runtime. Los cuatro IDs hash son proyecciones del adaptador; los demás usan ID persistido.

## 26. Distribución de responsabilidades

| Responsabilidad | Dominio Go | Application | PostgreSQL | Contrato/Bridge | Otro |
|---|---|---|---|---|---|
| taxonomía y reglas | structs, `Validate`, `NewResource` | orquesta autoridad | FKs/checks refuerzan | proyecta | — |
| identidad v1 | deriva/verifica | exige en reactivación | UQ/check/migración map | renombra a `IdentityV1` | — |
| lifecycle | métodos y sentinels | decide precondiciones | lock/transición/revision | shape + traducción | consumidor aporta expected rev |
| catálogo runtime | `ResourceCatalog`, `CatalogAuthority` | mutex, candidato, publicación | source de filas; loader snapshot | expone descriptors/records | composición no presente |
| persistencia recurso | puerto | canonicaliza | transacciones/codec/SQL | no accede DB | — |
| búsqueda | criteria y normalize | delega | SQL/CTE/hidratación | limita superficie pública | — |
| errores | sentinels | envuelve contexto | clasifica SQLSTATE | neutraliza en 15 códigos | — |
| actor | — | diagnóstico vía contexto | no persiste | exige y adjunta | no auth/audit encontrado |
| revisiones | tipos/puertos | exige >0 | columna/CAS | DTO/request | migración 8 pendiente viva |
| auditoría técnica | — | `core.Record` en V2 | marcas temporales/triggers | mensajes opacos | config logging |

## Hallazgos confirmados

1. Material es `Resource{ClassCode:"MATERIAL"}`, no agregado independiente.
2. Dominio, aplicación, PostgreSQL y frontera pública forman un Core hexagonal; no existe delivery/composition productiva en este checkout.
3. La base viva está en migración 4; HEAD llega a 8. Migración 6 es Supplier Master y no altera Resource Master.
4. El snapshot runtime de catálogo se carga desde DB con `RepeatableRead`, `ReadOnly` y doce queries físicas para once kinds.
5. Los cinco recursos vivos están activos, usan identidad `v1|...` y contienen 21 valores `SET`.
6. Resource CAS y Catalog CAS están implementados y probados; ambos requieren migración 8.
7. Todas las rutas públicas de write CAS exigen actor, ID no cero y expected revision no cero; sólo recursos vuelven a imponer ID positivo en aplicación.
8. El bridge tiene assertions compile-time para `ReadCapabilities` y `WriteCapabilities` y mappings de reglas/campos previamente omitidos.

## Discrepancias

1. DB viva sin `revision` frente a código HEAD que consulta esas columnas.
2. Lecturas ordinarias y create-confirm no cargan revisión, pero writers CAS exigen una positiva.
3. Update legacy recalcula identidad; update CAS conserva la anterior.
4. `ScopeAll` es válido en DTO/Reader de recursos pero el bridge lo rechaza.
5. Catálogo limit 0 devuelve hasta una fila en vez de un default consistente.
6. El público no expone filtros tipados de recursos, parent filters/dependencies de catálogo ni `LifecycleResult.Changed`.
7. Descriptor público omite metadatos internos de mutabilidad, búsqueda, ayuda, creación inline y dependencias.
8. Reader rechaza IDs no positivos; Writer sólo rechaza cero. Los IDs negativos de catálogo no se clasifican uniformemente como `INVALID_ARGUMENT`.

## Comportamiento aparentemente muerto

1. `CatalogAdminRepositoryV2` y su implementación existen; no se encontró composition root productivo que llame `WithCatalogAdminRepositoryV2`.
2. `ResetCatalogWriterAvailability` sólo es útil para una composición/operator reload no presente en este repositorio.
3. `ResourceRepository.SetActive` persiste como alias de compatibilidad; los casos de uso usan lifecycle explícito.
4. `recursos.Service.Delete` es alias soft-delete; no representa eliminación física.

## Comportamiento no protegido

1. `UpdateRevision` puede confirmar cambio de clase y fallar al recargar por clase anterior.
2. `ReactivateRevision` de recurso ya activo retorna antes de comprobar expected revision; stale revision puede tener éxito silencioso.
3. Lifecycle V2 de catálogo ya en estado objetivo retorna antes del repositorio; stale revision puede tener éxito silencioso.
4. Los confirm-read de create recurso y lifecycle catálogo son post-commit y no transaccionales.
5. `garfex_app` vivo tiene DELETE sobre Resource Master; la ausencia de `HardDeleteResource` protege sólo a través del Core, no contra SQL directo.
6. No se encontró E2E real Reader→bridge→PostgreSQL→primer write CAS con revisión obtenida por read.
7. No se encontró auditoría persistida de Actor, autenticación, autorización, tenant ni rate limiting.
8. `ResourceCatalog.Validate` no cubre toda la integridad declarada por los descriptores: duplicados de tipo, `suggested => allowed`, estructura de opciones/relaciones y presentación dependen de PostgreSQL o de pruebas de fixture.

## 27. Cobertura individual por invariante

“Integration test” indica que existe en el repositorio; no implica que se haya ejecutado contra la DB viva durante esta auditoría. “E2E” exige composición pública real con PostgreSQL, no fakes de frontera.

| ID | Unit test | Integration test | E2E | Sin cobertura |
|---|---|---|---|---|
| RM-INV-001 | `TestResourceCatalog_Validate_DefectCases` | `TestLoadResourceCatalogIntegration` | no | no |
| RM-INV-002 | `TestResourceCatalog_Validate_DefectCases` | `TestResourceIntegrityMigrationAuditRelation` | no | no |
| RM-INV-003 | `TestResourceCatalog_Validate_FamilyCodeReusedAcrossClassesIsValid` + defect case | `TestCatalogAdminRepositoryIntegration` | no | no |
| RM-INV-004 | no se encontró validador unitario de catálogo para duplicado de tipo | `TestCatalogCASCreateUpdateIntegration` / UQ SQL | no | parcial en Go |
| RM-INV-005 | dos tests de parent inactivo | `TestCatalogCASLifecycleDeleteIntegration` | no | no |
| RM-INV-006 | no se encontró rechazo unitario de suggested sin allowed | constraint ejercido por integración de catálogo | no | parcial en Go |
| RM-INV-007 | conductor/value tests | `TestResourceRepositoryIntegration` | no | no |
| RM-INV-008 | scope/query tests | `TestResourceRepositoryAttributeCardinalityIntegration` | no | no |
| RM-INV-009 | dos tests conditional iff rules | `TestLoadResourceCatalogIntegration` | no | no |
| RM-INV-010 | `TestSeedResourceCatalog_UnitNamesAndConditionalRuntimeFixture` | loader rules | no | no |
| RM-INV-011 | option-set y official-code tests | repository integration | no | no |
| RM-INV-012 | `TestValidOptionsNarrowsDiameter*` | repository integration | no | no |
| RM-INV-013 | tres tests comprueban consistencia de fixture, no `Validate` | catalog V2 integration + constraints | no | parcial en Go |
| RM-INV-014 | canonical write service tests | repository integration | no | no |
| RM-INV-015 | `TestConductorValidationRejectsMissingForbiddenInvalidAndUnitValues` | repository integration | no | no |
| RM-INV-016 | `TestEncodeAttributeValueRejectsNotApplicableWithPayload` | repository integration | no | no |
| RM-INV-017 | conductor/unit tests | repository integration | no | no |
| RM-INV-018 | `TestNewResourceDerivesV1IdentityFromSortedCanonicalParts` | integrity migration integration | no | no |
| RM-INV-019 | `TestIdentityComponentPreventsDelimiterCollision` | `TestResourceIntegrityMigrationIntegration` | no | no |
| RM-INV-020 | dos `NaturalUnitDoesNotParticipateInIdentity` | repository integration | no | no |
| RM-INV-021 | duplicate identity tests | repository integration | no | no |
| RM-INV-022 | `TestRehydrateResourcePreservesHistoryAfterCatalogDeactivation` | lifecycle integration | no | no |
| RM-INV-023 | reactivation service tests | lifecycle integration | no | no |
| RM-INV-024 | `TestResourceLifecycleMethodsAreExplicitAndIdempotent` | lifecycle integration | no | no |
| RM-INV-025 | lifecycle service tests | lifecycle integration | no | no |
| RM-INV-026 | service no-op tests | lifecycle/catalog CAS integration | no | no |
| RM-INV-027 | service CAS classification | concurrent revision integration | no | no |
| RM-INV-028 | service full replace | `TestResourceRepositoryUpdateRevisionAtomicCAS` | no | no |
| RM-INV-029 | query/cardinality unit | search + cardinality integration | no | no |
| RM-INV-030 | `TestSearchCriteriaNormalizeBoundsPageInputs` | search set hydration integration | no | no |
| RM-INV-031 | `TestServiceCreateRejectsInvalidMutationWithoutPersisting` | catalog CAS integration | no | no |
| RM-INV-032 | publication service tests | catalog CAS integration | no | no |
| RM-INV-033 | immutable/delete guard service tests | immutable/dependency race integration | no | no |
| RM-INV-034 | `TestPublicErrors`; bridge category tables | no | no | no |
| RM-INV-035 | Reader/Writer copy tests | no | no | no |
| RM-INV-036 | `TestWriter_NoUngraduatedMethodExported` | no | public fake only | no |
| RM-INV-037 | `TestServiceCanonicalWritesAndStableUpdateID` | repository integration | no | no |
| RM-INV-038 | service CAS tests | `TestResourceRepositoryUpdateRevisionAtomicCAS` | no | no |
| RM-INV-039 | Writer shape tests | lifecycle integration defensa | no | no |
| RM-INV-040 | evidencia negativa en bridge/reader tests | no | no | **sí: falta E2E read→CAS** |
| RM-INV-041 | migración revision tests en esquema migrado | `TestMigrationRevisionColumnsIntegration` | no | **sí en DB viva** |
| RM-INV-042 | `TestAdapter_SearchResourcesAllScopeNotSupported` | no | no | no; brecha fijada por test |
| RM-INV-043 | `TestAdapter_ListCatalogPagination` no cubre explícitamente limit 0 | no | no | **sí: caso limit 0** |
| RM-INV-044 | no | update CAS cubre otros cambios | no | **sí: cambio de clase + reload post-commit** |

## 28. Recuentos verificables y cierre

| Elemento | Recuento | Método de verificación |
|---|---:|---|
| tablas Resource Master en DB viva | 14 | tablas de migraciones 000002–000004; más `schema_migrations` = 15 públicas observadas |
| tablas Resource Master en HEAD | 15 | 14 anteriores + `resource_integrity_identity_map`; 000008 no crea tablas |
| kinds administrables | 11 | `NewCatalogRegistry`; `TestCatalogRegistry_HasElevenKinds` |
| métodos exportados de `recursos.Service` | 12 | firmas `func (s *Service)` en `internal/app/recursos/service.go` |
| métodos exportados de `catalogo.Service` | 18 | firmas en `internal/app/catalogo/service.go` |
| operaciones públicas | 16 | 7 `ReadCapabilities` + 9 `WriteCapabilities` |
| invariantes ejecutables | 39 | RM-INV-001–039 |
| expectativas incumplidas | 5 | RM-INV-040–044, separadas en §19.1 |
| casos requeridos | 14 | §14.1, exactamente numerados 1–14 |
| funciones de prueba inventariadas | 338 | `rg '^func Test'`: domain 130 + app recursos 22 + app catálogo 36 + postgres 39 + resourcecore 45 + bridge 66 |

**Conclusión factual.** El Resource Master actual tiene una autoridad de dominio rica y un adaptador PostgreSQL con garantías transaccionales reales. La equivalencia funcional futura no puede evaluarse sólo por la presencia de CRUD: debe contemplar los 11 kinds, reglas condicionales, identidad v1, lectura histórica, lifecycle, búsqueda tipada interna, publicación coherente y los defectos concretos de revisión/composición aquí separados de los invariantes ejecutables.

## 29. Esquema PostgreSQL exacto por tabla

### 29.1 Convenciones del inventario exacto

- Las columnas descritas son las resultantes de aplicar las migraciones Resource Master de HEAD hasta `000008`; la columna “Viva” identifica la diferencia observada en migración 4.
- `NO ACTION` significa que la FK no declara una acción especial; sólo se indica `CASCADE` cuando el SQL lo declara.
- Cada `PRIMARY KEY` y `UNIQUE` crea su índice B-tree implícito. Sólo se nombran por separado los índices creados mediante `CREATE INDEX` o `CREATE UNIQUE INDEX`.
- Cuando una restricción se declaró sin `CONSTRAINT nombre`, el SQL fuente no fija un nombre contractual; se documenta su expresión y no se infiere un nombre.
- Todas las columnas `revision` de HEAD son `BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)`; no existe trigger de incremento. Las mutaciones CAS incrementan explícitamente la columna.

### 29.2 `resource_classes`

**Propósito:** raíz taxonómica de todo recurso.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `code` | `TEXT` | NOT NULL | no | código natural | sí |
| `name` | `TEXT` | NOT NULL | no | nombre singular | sí |
| `plural` | `TEXT` | NOT NULL | no | nombre plural | sí |
| `slug` | `TEXT` | NOT NULL | no | slug público | sí |
| `display_order` | `INTEGER` | NOT NULL | `0` | orden | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `aliases` | `TEXT[]` | NOT NULL | `'{}'` | alias | sí |
| `keywords` | `TEXT[]` | NOT NULL | `'{}'` | búsqueda | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; UNIQUE `code`; UNIQUE `slug`; CHECK `btrim(code) <> ''`, `btrim(name) <> ''`, `btrim(plural) <> ''`, `btrim(slug) <> ''`, `revision > 0`. FKs: ninguna. Índices explícitos: ninguno. Cascadas: ninguna. Trigger `resource_classes_set_updated_at`, `BEFORE UPDATE FOR EACH ROW`, ejecuta `set_updated_at()`. `active`: sí. `created_at`/`updated_at`: sí. CAS: columna sólo en HEAD.

### 29.3 `resource_option_sets`

**Propósito:** namespace natural de opciones controladas.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `code` | `TEXT` | NOT NULL | no | PK natural | sí |
| `name` | `TEXT` | NOT NULL | no | nombre | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `code`; CHECK `btrim(code) <> ''`, `btrim(name) <> ''`, `revision > 0`. FKs: ninguna. UNIQUE adicional: no. Índices explícitos: no. Cascadas: no. Trigger `resource_option_sets_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.4 `resource_families`

**Propósito:** familia perteneciente a una clase.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `class_id` | `BIGINT` | NOT NULL | no | FK clase | sí |
| `code` | `TEXT` | NOT NULL | no | código en clase | sí |
| `name` | `TEXT` | NOT NULL | no | nombre | sí |
| `description` | `TEXT` | NOT NULL | `''` | descripción | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; FK `class_id -> resource_classes(id)` `NO ACTION`; UNIQUE `(class_id, code)`; constraint nombrada `resource_families_id_class_key` UNIQUE `(id, class_id)`; CHECK no blank sobre `code` y `name`, más `revision > 0`. Índices explícitos: no. Cascadas: no. Trigger `resource_families_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.5 `resource_types`

**Propósito:** tipo perteneciente simultáneamente a familia y clase coherentes.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `class_id` | `BIGINT` | NOT NULL | no | scope clase | sí |
| `family_id` | `BIGINT` | NOT NULL | no | scope familia | sí |
| `code` | `TEXT` | NOT NULL | no | código en familia | sí |
| `name` | `TEXT` | NOT NULL | no | nombre | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; FK compuesta `(family_id, class_id) -> resource_families(id, class_id)` `NO ACTION`; UNIQUE `(family_id, code)`; constraint `resource_types_id_family_key` UNIQUE `(id, family_id)`; CHECK no blank sobre `code` y `name`, más `revision > 0`. Índices explícitos: no. Cascadas: no. Trigger `resource_types_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.6 `unit_definitions`

**Propósito:** unidad utilizable como unidad natural o unidad de cantidad.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `code` | `TEXT` | NOT NULL | no | código único | sí |
| `symbol` | `TEXT` | NOT NULL | no | símbolo único | sí |
| `dimension` | `TEXT` | NOT NULL | no | dimensión | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `name` | `TEXT` | NOT NULL | no | nombre humano, agregado en 000004 | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; UNIQUE `code`; UNIQUE `symbol`; CHECK no blank sobre `code`, `symbol`, `dimension`; constraint `unit_definitions_name_nonblank` CHECK `btrim(name) <> ''`; CHECK `revision > 0`. FKs: no. Índices explícitos: no. Cascadas: no. Trigger `unit_definitions_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.7 `resource_unit_policies`

**Propósito:** habilitación y sugerencia de una unidad para una familia.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `family_id` | `BIGINT` | NOT NULL | no | PK/FK familia | sí |
| `unit_id` | `BIGINT` | NOT NULL | no | PK/FK unidad | sí |
| `allowed` | `BOOLEAN` | NOT NULL | `TRUE` | permite uso | sí |
| `suggested` | `BOOLEAN` | NOT NULL | `FALSE` | preferencia | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `(family_id, unit_id)`; FK `family_id -> resource_families(id)` y FK `unit_id -> unit_definitions(id)`, ambas `NO ACTION`; CHECK `NOT suggested OR allowed`; CHECK `revision > 0`. UNIQUE adicional: no. Índices explícitos: no. Cascadas: no. Trigger `resource_unit_policies_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.8 `attribute_definitions`

**Propósito:** definición global y tipada de una característica.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `code` | `TEXT` | NOT NULL | no | código global | sí |
| `name` | `TEXT` | NOT NULL | no | nombre | sí |
| `value_type` | `TEXT` | NOT NULL | no | discriminante del payload | sí |
| `dimension` | `TEXT` | NULL | no | dimensión opcional | sí |
| `default_identity_participates` | `BOOLEAN` | NOT NULL | `FALSE` | default identitario | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; UNIQUE `code`; CHECK no blank sobre `code` y `name`; CHECK `value_type IN ('CONTROLLED_OPTION','INTEGER','DECIMAL','QUANTITY','BOOLEAN','CONTROLLED_TEXT')`; CHECK `revision > 0`. FKs: no. Índices explícitos: no. Cascadas: no. Trigger `attribute_definitions_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.9 `attribute_options`

**Propósito:** opción controlada dentro de conjunto y característica.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `option_set` | `TEXT` | NOT NULL | `'DEFAULT'` | PK/FK conjunto | sí |
| `attribute_definition_id` | `BIGINT` | NOT NULL | no | PK/FK característica | sí |
| `code` | `TEXT` | NOT NULL | no | PK/código | sí |
| `label` | `TEXT` | NOT NULL | no | etiqueta | sí |
| `numeric_value` | `NUMERIC` | NULL | no | equivalencia numérica | sí |
| `unit_id` | `BIGINT` | NULL | no | unidad de equivalencia | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `display_order` | `INTEGER` | NOT NULL | `0` | orden en parent | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `(option_set, attribute_definition_id, code)`; FK `option_set -> resource_option_sets(code)`, FK `attribute_definition_id -> attribute_definitions(id)`, FK nullable `unit_id -> unit_definitions(id)`, todas `NO ACTION`; CHECK no blank sobre `code` y `label`; CHECK `(numeric_value IS NULL) = (unit_id IS NULL)`; constraint `attribute_options_display_order_key` UNIQUE `(option_set, attribute_definition_id, display_order)`; CHECK `revision > 0`. Índices explícitos: no. Cascadas: no. Trigger `attribute_options_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.10 `attribute_option_relations`

**Propósito:** relación dirigida entre dos opciones del mismo conjunto.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `option_set` | `TEXT` | NOT NULL | `'DEFAULT'` | FK conjunto y parte de FKs compuestas | sí |
| `from_attribute_definition_id` | `BIGINT` | NOT NULL | no | definición origen | sí |
| `from_option_code` | `TEXT` | NOT NULL | no | opción origen | sí |
| `to_attribute_definition_id` | `BIGINT` | NOT NULL | no | definición destino | sí |
| `to_option_code` | `TEXT` | NOT NULL | no | opción destino | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; UNIQUE `(option_set, from_attribute_definition_id, from_option_code, to_attribute_definition_id, to_option_code)`; FK `option_set -> resource_option_sets(code)`; FK compuesta origen y FK compuesta destino hacia `attribute_options(option_set, attribute_definition_id, code)`, todas `NO ACTION`; CHECK no blank sobre ambos códigos de opción; CHECK `from_attribute_definition_id <> to_attribute_definition_id`; CHECK `revision > 0`. Índices explícitos: no. Cascadas: no. Trigger `attribute_option_relations_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.11 `resource_attributes`

**Propósito:** binding de característica aplicable a familia o tipo; parent de reglas.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `class_id` | `BIGINT` | NOT NULL | no | scope clase | sí |
| `family_id` | `BIGINT` | NOT NULL | no | scope familia | sí |
| `type_id` | `BIGINT` | NULL | no | scope tipo opcional | sí |
| `definition_id` | `BIGINT` | NOT NULL | no | FK característica | sí |
| `option_set` | `TEXT` | NOT NULL | `'DEFAULT'` | FK conjunto | sí |
| `mode` | `TEXT` | NOT NULL | no | modo base | sí |
| `identity_participates` | `BOOLEAN` | NOT NULL | `FALSE` | identidad base | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `display_order` | `INTEGER` | NOT NULL | `0` | orden | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS del agregado, incluidas reglas | no |

PK `id`; FK `definition_id -> attribute_definitions(id)`; FK `option_set -> resource_option_sets(code)`; FK compuesta `(family_id,class_id) -> resource_families(id,class_id)`; FK compuesta nullable `(type_id,family_id) -> resource_types(id,family_id)`, todas `NO ACTION`; CHECK `mode IN ('REQUIRED','OPTIONAL','CONDITIONAL','FORBIDDEN')`; CHECK `revision > 0`; constraint `resource_attributes_id_family_definition_key` UNIQUE `(id,family_id,definition_id)`. Índices explícitos: `resource_attributes_family_definition_key` UNIQUE `(family_id,definition_id) WHERE type_id IS NULL`; `resource_attributes_type_definition_key` UNIQUE `(family_id,type_id,definition_id) WHERE type_id IS NOT NULL`; `resource_attributes_family_display_order_key` UNIQUE `(family_id,display_order) WHERE type_id IS NULL`; `resource_attributes_type_display_order_key` UNIQUE `(family_id,type_id,display_order) WHERE type_id IS NOT NULL`. Cascadas: no. Trigger `resource_attributes_set_updated_at`. Las cuatro columnas inline de condición de 000002 y sus checks/FK fueron eliminados por 000003. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.12 `resource_attribute_rules`

**Propósito:** reglas ordenadas del agregado `APLICABILIDAD`.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `resource_attribute_id` | `BIGINT` | NOT NULL | no | FK parent | sí |
| `display_order` | `INTEGER` | NOT NULL | no | orden | sí |
| `when_definition_id` | `BIGINT` | NOT NULL | no | antecedente | sí |
| `when_equals` | `TEXT` | NOT NULL | no | valor antecedente | sí |
| `mode` | `TEXT` | NOT NULL | no | modo resultante | sí |
| `identity_participates` | `BOOLEAN` | NOT NULL | `FALSE` | identidad resultante | sí |
| `not_applicable` | `BOOLEAN` | NOT NULL | `FALSE` | marca N/A | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |

PK `id`; FK `resource_attribute_id -> resource_attributes(id) ON DELETE CASCADE`; FK `when_definition_id -> attribute_definitions(id)` `NO ACTION`; UNIQUE `(resource_attribute_id, display_order)`; CHECK `btrim(when_equals) <> ''`; CHECK `mode IN ('REQUIRED','OPTIONAL','CONDITIONAL','FORBIDDEN')`. Índices explícitos: no. Trigger `resource_attribute_rules_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. `revision`: no; la revisión pertenece al parent `resource_attributes`.

### 29.13 `resource_type_presentation_fields`

**Propósito:** campos y orden de presentación de un tipo.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `type_id` | `BIGINT` | NOT NULL | no | PK/FK tipo | sí |
| `attribute_definition_id` | `BIGINT` | NOT NULL | no | PK/FK característica | sí |
| `position` | `INTEGER` | NOT NULL | no | posición única | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `(type_id, attribute_definition_id)`; FK `type_id -> resource_types(id)` y FK `attribute_definition_id -> attribute_definitions(id)`, ambas `NO ACTION`; UNIQUE `(type_id, position)`; CHECK `revision > 0`. Índices explícitos: no. Cascadas: no. Trigger `resource_type_presentation_fields_set_updated_at`. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.14 `recursos`

**Propósito:** parent persistido de cada recurso maestro.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `class_id` | `BIGINT` | NOT NULL | no | FK clase | sí |
| `family_id` | `BIGINT` | NOT NULL | no | FK familia | sí |
| `type_id` | `BIGINT` | NOT NULL | no | FK tipo | sí |
| `natural_unit_id` | `BIGINT` | NOT NULL | no | FK unidad natural | sí |
| `display_name` | `TEXT` | NOT NULL | no | presentación persistida | sí |
| `identity_key` | `TEXT` | NOT NULL | no | identidad durable | sí |
| `active` | `BOOLEAN` | NOT NULL | `TRUE` | lifecycle | sí |
| `created_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | creación técnica | sí |
| `updated_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | última modificación | sí |
| `revision` | `BIGINT` | NOT NULL | `1` | CAS | no |

PK `id`; FK `class_id -> resource_classes(id)`; FK `natural_unit_id -> unit_definitions(id)`; FK compuesta `(family_id,class_id) -> resource_families(id,class_id)`; FK compuesta `(type_id,family_id) -> resource_types(id,family_id)`, todas `NO ACTION`; UNIQUE `(class_id, identity_key)`; constraint `recursos_id_family_key` UNIQUE `(id,family_id)`; CHECK no blank sobre `display_name` e `identity_key`; constraint `recursos_identity_key_v1` CHECK `identity_key LIKE 'v1|%'` desde 000007; CHECK `revision > 0`. Índice explícito `recursos_class_id_idx` sobre `(class_id)`. Cascadas: no. Trigger de `updated_at`: **no**; los repositorios actualizan `updated_at` explícitamente. `active`: sí. `created_at`/`updated_at`: sí. CAS: sólo HEAD.

### 29.15 `resource_attribute_values`

**Propósito:** unión tipada de valores pertenecientes a un recurso y binding efectivo.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `id` | `BIGSERIAL` | NOT NULL | secuencia | PK | sí |
| `resource_id` | `BIGINT` | NOT NULL | no | FK recurso | sí |
| `family_id` | `BIGINT` | NOT NULL | no | scope compuesto | sí |
| `resource_attribute_id` | `BIGINT` | NOT NULL | no | FK binding | sí |
| `attribute_definition_id` | `BIGINT` | NOT NULL | no | scope compuesto | sí |
| `option_set` | `TEXT` | NOT NULL | `'DEFAULT'` | FK conjunto/opción | sí |
| `value_state` | `TEXT` | NOT NULL | `'SET'` | `SET` o `NOT_APPLICABLE` | sí |
| `option_code` | `TEXT` | NULL | no | payload opción | sí |
| `integer_value` | `BIGINT` | NULL | no | payload entero | sí |
| `decimal_value` | `NUMERIC` | NULL | no | payload decimal | sí |
| `quantity_value` | `NUMERIC` | NULL | no | payload cantidad | sí |
| `quantity_unit_id` | `BIGINT` | NULL | no | unidad de cantidad | sí |
| `boolean_value` | `BOOLEAN` | NULL | no | payload booleano | sí |
| `text_value` | `TEXT` | NULL | no | payload texto | sí |

PK `id`; UNIQUE `(resource_id, resource_attribute_id)`; FK `(resource_id,family_id) -> recursos(id,family_id) ON DELETE CASCADE`; FK `(resource_attribute_id,family_id,attribute_definition_id) -> resource_attributes(id,family_id,definition_id)` `NO ACTION`; FK `(option_set,attribute_definition_id,option_code) -> attribute_options(option_set,attribute_definition_id,code)` `NO ACTION`; FK `option_set -> resource_option_sets(code)` `NO ACTION`; FK nullable `quantity_unit_id -> unit_definitions(id)` `NO ACTION`. CHECK `value_state IN ('SET','NOT_APPLICABLE')`; para `SET`, exactamente un payload no nulo; para `NOT_APPLICABLE`, cero payloads; cantidad `SET` exige `quantity_unit_id`; texto `SET` no puede quedar en blanco. Índices explícitos: no. Trigger `resource_attribute_values_validate_type`, `BEFORE INSERT OR UPDATE`, ejecuta `validate_resource_attribute_value()` y exige payload de opción sólo para `CONTROLLED_OPTION`. `active`: no. `created_at`/`updated_at`: no. `revision`: no.

### 29.16 `resource_integrity_identity_map`

**Propósito:** mapa de transición entre identidad legacy e identidad v1, creado por 000005.

| Columna | Tipo | Nulabilidad | Default | Rol | Viva |
|---|---|---|---|---|---|
| `resource_id` | `BIGINT` | NOT NULL | no | PK/FK recurso | ausente |
| `class_id` | `BIGINT` | NOT NULL | no | FK clase/scope unique | ausente |
| `legacy_identity_key` | `TEXT` | NOT NULL | no | identidad previa | ausente |
| `v1_identity_key` | `TEXT` | NOT NULL | no | identidad canónica | ausente |
| `mapped_at` | `TIMESTAMPTZ` | NOT NULL | `NOW()` | momento de mapeo | ausente |

PK/FK `resource_id -> recursos(id) ON DELETE CASCADE`; FK `class_id -> resource_classes(id)` `NO ACTION`; UNIQUE `(class_id, legacy_identity_key)`; UNIQUE `(class_id, v1_identity_key)`. CHECK: no. Índices explícitos: no. Triggers: no. `active`: no. `created_at`/`updated_at`: no; sólo `mapped_at`. `revision`: no. La tabla completa está ausente en la base viva migración 4.

## 30. Operaciones exactas, una por una

### 30.1 Casos de uso internos de recursos

| Operación | Propósito | Entrada | Salida | Lecturas | Escrituras | Validaciones/reglas | Transacción/locks | Errores | Concurrencia | Implementación principal |
|---|---|---|---|---|---|---|---|---|---|---|
| `Get` | cargar recurso histórico por identidad | contexto, clase, identity v1 | `Resource` | `recursos`, catálogo de scope y valores | ninguna | claves no vacías; hidratación e identidad v1 | ninguna; lectura ordinaria | invalid argument, not found, integrity, error envuelto | sin lock; puede observar commit concurrente | `recursos.Service.Get`; `resourceRepository.Get` |
| `Search` | búsqueda interna sin metadatos | `SearchCriteria` | `[]Resource` | las mismas dos consultas de `SearchPage` | ninguna | delega normalización/repositorio | ninguna | errores de búsqueda envueltos | snapshot por statement en Read Committed | `recursos.Service.Search`; `resourceRepository.Search` |
| `SearchPage` | buscar página estable | `SearchCriteria` | `ResourcePage` | parent `limit+1` y carga set-based de valores | ninguna | límite 0→50, máximo 50, offset no negativo, binding efectivo único | ninguna | invalid argument, integrity, SQL envuelto | sin locks; dos statements no forman snapshot repetible | `recursos.Service.SearchPage`; `resourceRepository.SearchPage` |
| `Describe` | producir presentación canónica | `Resource` | `string` | snapshot in-memory de autoridad | ninguna | orden de presentación; omite ausentes/N/A | ninguna | ninguno; fallback o cadena vacía | versión de catálogo obtenida al invocar | `recursos.Service.Describe`; `ResourceCatalog.Describe` |
| `Create` | crear recurso canónico | `CreateCommand` | candidato `Resource` sin ID/revisión persistidos | snapshot; resolución activa de scope, unidad, bindings y opciones; conteo final | INSERT `recursos`; INSERT N valores | `NewResource`; canonical; refs activas; identidad; cardinalidad | tx Read Committed; sin row lock; rollback diferido | validation, duplicate, reference, integrity, SQL envuelto | unique resuelve carrera de identidad | `recursos.Service.Create`; `resourceRepository.Create` |
| `Update` | reemplazo completo legacy | `UpdateCommand` | candidato canónico | snapshot; scope/unidad/bindings; fila target; conteo | UPDATE parent con identidad nueva; DELETE e INSERT valores | ID positivo; mismas reglas de create | tx Read Committed; sin `FOR UPDATE` explícito | not found, duplicate, reference, integrity | last-writer-wins; sin token CAS | `recursos.Service.Update`; `resourceRepository.Update` |
| `Deactivate` | pasar a inactivo | ID | `LifecycleResult` | fila `FOR UPDATE`; `Get` post-commit | UPDATE `active=false` sólo si cambia | ID positivo; existencia | tx Read Committed + row lock | not found o error envuelto | serializa lifecycle sobre fila | `recursos.Service.Deactivate`; `setLifecycle` |
| `Delete` | alias compatible de desactivación | ID | error | las de `Deactivate` | las de `Deactivate`; nunca DELETE | las de `Deactivate` | la tx de `Deactivate` | invalid argument, not found o wrapper adicional | igual a `Deactivate` | `recursos.Service.Delete` |
| `Reactivate` | restaurar bajo catálogo actual | ID | `LifecycleResult` | `GetByID`; snapshot; fila `FOR UPDATE`; refs activas; `Get` post-commit | UPDATE `active=true` si cambia | ID positivo; canonicalización; identidad exacta; refs activas | prelecturas fuera de tx; tx Read Committed + row lock | not found, duplicate, identity conflict, reactivation impossible | lock protege transición; autoridad puede cambiar entre prevalidación y tx | `recursos.Service.Reactivate`; `setLifecycle` |
| `DeactivateRevision` | desactivar con CAS | ID, expected revision | `LifecycleResult` | fila con revisión `FOR UPDATE`; `getWithRevision` post-commit | UPDATE active y `revision+1` si cambia | ID/revisión positivos; comparación exacta | tx Read Committed + row lock | not found, conflict, error envuelto | stale writer falla tras lock; no retry | `recursos.Service.DeactivateRevision`; `setLifecycleRevision` |
| `ReactivateRevision` | reactivar con catálogo, identidad y CAS | ID, expected revision | `LifecycleResult` | `GetByID`; snapshot; fila con revisión `FOR UPDATE`; refs; recarga | UPDATE active y `revision+1` si cambia | mismas reglas de reactivación; expected revision | prelectura fuera de tx; tx Read Committed + row lock | conflict, not found, identity conflict, reactivation impossible | si ya está activo, servicio retorna antes del CAS | `recursos.Service.ReactivateRevision`; `setLifecycleRevision` |
| `UpdateRevision` | reemplazo completo con CAS | `UpdateCommand`, expected revision | recurso recargado con revisión | snapshot; fila target `FOR UPDATE`; scope/bindings; recarga post-commit | UPDATE parent sin identity; `revision+1`; DELETE/INSERT valores | ID/revisión positivos; canonical; cardinalidad | tx Read Committed + row lock | conflict, not found, duplicate, reference, integrity; posible error post-commit | stale writer falla; cambio de clase puede confirmar y fallar al recargar | `recursos.Service.UpdateRevision`; `resourceRepository.UpdateRevision` |

### 30.2 Casos de uso internos de catálogo

| Operación | Propósito | Entrada | Salida | Lecturas | Escrituras | Validaciones/reglas | Transacción/locks | Errores | Concurrencia | Implementación principal |
|---|---|---|---|---|---|---|---|---|---|---|
| `Kinds` | devolver registro fijo | ninguna | 11 `CatalogKind` | memoria | ninguna | copia defensiva del slice superior | mutex: no; tx: ninguna | ninguno | registro inmutable | `catalogo.Service.Kinds` |
| `List` | listar registros administrativos | kind, `CatalogFilter` | records | handler SQL del kind | ninguna | kind conocido; status/text/parent | ninguna | kind unknown, SQL | sin lock | `catalogo.Service.List`; repo `List` |
| `Get` | obtener record por ID | kind, ID | record | handler SQL del kind | ninguna | ID distinto de cero | ninguna | invalid argument, kind unknown, not found, SQL | sin lock | `catalogo.Service.Get`; repo `Get` |
| `Dependencies` | contar referencias hijas | kind, ID | dependencias | probes declarados por handler | ninguna | handler conocido | ninguna | kind unknown, SQL | valor puntual sin lock | `catalogo.Service.Dependencies`; repo `Dependents` |
| `ReferencedByResources` | detectar uso por recursos | kind, ID | bool | probe de recursos o retorno falso para junction | ninguna | handler conocido | ninguna | kind unknown, SQL | valor puntual sin lock | `catalogo.Service.ReferencedByResources` |
| `Create` | crear y publicar catálogo | kind, record | record con ID | snapshot; con V2 recarga completa | legacy INSERT o V2 INSERT | mutation + `Validate`; selecciona V2 si configurado | mutex de servicio; legacy handler/tx; V2 tx coherente | invalid catalog, duplicate, reference, unavailable/indeterminate | serialización sólo in-process; DB constraints entre procesos | `catalogo.Service.Create` |
| `Update` | reemplazar record legacy | kind, record con ID | replacement | current; descriptor; uso por recursos; snapshot | UPDATE del handler | ID no cero; código inmutable si referenciado; mutation + Validate | mutex de servicio; tx de repo | invalid argument, immutable, invalid catalog, not found, duplicate/reference | sin CAS; serializado sólo en proceso | `catalogo.Service.Update` |
| `Deactivate` | lifecycle legacy a inactivo | kind, ID | error | current y snapshot | UPDATE active | ID no cero; mutation + Validate jerárquico | mutex + tx de repo | invalid argument, not found, invalid catalog/reference | sin CAS | `catalogo.Service.Deactivate`; `setActive` |
| `Reactivate` | lifecycle legacy a activo | kind, ID | error | current y snapshot | UPDATE active | ID no cero; catálogo completo válido | mutex + tx de repo | invalid argument, not found, invalid catalog/reference | sin CAS | `catalogo.Service.Reactivate`; `setActive` |
| `Delete` | hard-delete legacy conservador | kind, ID | error | current, dependencias, referencias, snapshot | DELETE | ID; target inactivo; cero dependencias/refs; candidato válido | mutex + tx de repo | invalid lifecycle, in use, not found, invalid catalog | probes y DELETE no comparten una única tx de servicio; FK es backstop | `catalogo.Service.Delete` |
| `WithCatalogAdminRepositoryV2` | configurar writer V2 | repo V2 | mismo servicio | ninguna | asigna campo in-memory | ninguna comprobación nil | mutex; ninguna tx | ninguno | serializa asignación | `catalogo.Service.WithCatalogAdminRepositoryV2` |
| `CatalogAdminRepositoryV2Configured` | consultar composición | ninguna | bool | campo in-memory | ninguna | ninguna | mutex; ninguna tx | ninguno | lectura serializada | `catalogo.Service.CatalogAdminRepositoryV2Configured` |
| `ResetCatalogWriterAvailability` | limpiar latch operatorio | ninguna | ninguna | ninguna | `writerUnavailable=false` | requiere coherencia externa por contrato, no la verifica | mutex; ninguna tx | ninguno | serializa reset | `catalogo.Service.ResetCatalogWriterAvailability` |
| `CreateV2` | crear por writer coherente | kind, record | record coherente | snapshot; recarga completa en tx de repo | INSERT | repo V2 configurado/no latched; mutation + Validate | mutex + tx V2 | unavailable, invalid catalog, duplicate/reference, indeterminate | CAS no aplica a insert; constraints resuelven carrera | `catalogo.Service.CreateV2`; `insertLocked` |
| `UpdateRevision` | reemplazar catálogo con CAS | kind, record, expected revision | record coherente | current; descriptor; refs; snapshot; recarga completa | UPDATE con revisión | ID/revisión no cero; immutability; candidato válido | mutex + tx V2; CAS SQL/lock según handler | conflict, immutable, unavailable, indeterminate, repo errors | stale revision no publica | `catalogo.Service.UpdateRevision` |
| `DeactivateRevision` | desactivar catálogo con CAS | kind, ID, expected revision | error | current; snapshot; recarga si cambia | UPDATE active/revision si cambia | ID/revisión no cero; candidato válido | mutex + tx V2 si cambia | conflict, invalid catalog, unavailable | no-op retorna antes de comprobar revisión | `catalogo.Service.DeactivateRevision`; `setActiveRevision` |
| `ReactivateRevision` | reactivar catálogo con CAS | kind, ID, expected revision | error | current; snapshot; recarga si cambia | UPDATE active/revision si cambia | ID/revisión no cero; catálogo completo válido | mutex + tx V2 si cambia | conflict, invalid catalog, unavailable | no-op retorna antes de comprobar revisión | `catalogo.Service.ReactivateRevision`; `setActiveRevision` |
| `HardDeleteRevision` | hard-delete catálogo con CAS | kind, ID, expected revision | error | current, dependencias, refs, snapshot; recarga completa | DELETE | ID/revisión; target inactivo; sin uso; candidato válido | mutex + tx V2 | conflict, invalid lifecycle, in use, unavailable/indeterminate | FK y CAS son backstop de carrera | `catalogo.Service.HardDeleteRevision` |

### 30.3 Operaciones públicas de lectura

| Operación | Propósito | Entrada | Salida | Lecturas | Escrituras | Validaciones/reglas | Transacción/locks | Errores | Concurrencia | Implementación principal |
|---|---|---|---|---|---|---|---|---|---|---|
| `ActiveClasses` | clases activas | contexto | records | catálogo `List(CLASE, active)` | ninguna | copia defensiva | ninguna | error público ya neutralizado | lectura ordinaria | `Reader.ActiveClasses`; `Adapter.ActiveClasses` |
| `CatalogDescriptors` | descriptors de 11 kinds | contexto | descriptors | registro in-memory | ninguna | copia defensiva; bridge omite cuatro metadatos de field y grafo lifecycle | ninguna | ninguno esperado | sin estado mutable expuesto | `Reader.CatalogDescriptors`; bridge |
| `ListCatalog` | página de catálogo | `CatalogQuery` | `CatalogPage` | repo List | ninguna | kind/scope conocidos; bridge no expone Parent | ninguna | invalid argument o neutralizado | limit 0 pide una fila y oculta next | `Reader.ListCatalog`; `Adapter.ListCatalog` |
| `GetCatalog` | record por kind+ID | `CatalogKey` | record | repo Get | ninguna | kind conocido; ID positivo | ninguna | invalid argument/not found/neutralizado | sin lock; revision ordinaria queda 0 | `Reader.GetCatalog`; bridge |
| `SearchResources` | página pública de recursos | `ResourceQuery` | `ResourcePage` | búsqueda parent+valores | ninguna | scope conocido; bridge rechaza `ScopeAll`; sin filtros tipados | ninguna | invalid argument/not supported/neutralizado | dos statements sin snapshot común | `Reader.SearchResources`; bridge |
| `GetResource` | recurso por clase+identidad | `ResourceKey` | recurso | repo Get | ninguna | class e identity no vacíos | ninguna | invalid argument/not found/neutralizado | lectura ordinaria, revision 0 | `Reader.GetResource`; bridge |
| `DescribeResource` | descripción del recurso identificado | `ResourceKey` | string | Get de recurso + snapshot | ninguna | key válido; reglas de Describe | ninguna | invalid argument/not found/neutralizado | DB y catálogo pueden corresponder a instantes distintos | `Reader.DescribeResource`; bridge |

### 30.4 Operaciones públicas de escritura

| Operación | Propósito | Entrada | Salida | Lecturas | Escrituras | Validaciones/reglas | Transacción/locks | Errores | Concurrencia | Implementación principal |
|---|---|---|---|---|---|---|---|---|---|---|
| `CreateCatalog` | crear record genérico | actor, kind, active, values, rules | record | descriptor/snapshot; posible recarga V2 | INSERT | actor; kind; values; union/rules; dominio completo | tx interna legacy/V2; mutex catálogo | 15 categorías públicas | unique entre procesos; V2 sólo si compuesto | `Writer.CreateCatalog`; `Adapter.CreateCatalog` |
| `CreateResource` | crear recurso | actor, scope, unidad, atributos | recurso | snapshot; refs; Get confirmatorio | INSERT parent+valores | actor, scope/unidad; shape; dominio | tx de create; confirm-read fuera de tx | categorías públicas | unique identidad; confirmación no atómica y revision 0 | `Writer.CreateResource`; `Adapter.CreateResource` |
| `UpdateCatalog` | reemplazo CAS de catálogo | actor, kind, ID, revision, values/rules | record | current/refs/snapshot/recarga | UPDATE | actor; ID/revisión no cero; kind/shape; candidato | mutex + tx V2 | conflict y demás categorías | no-op no aplica; stale falla | `Writer.UpdateCatalog`; `Adapter.UpdateCatalog` |
| `UpdateResource` | reemplazo CAS de recurso | actor, ID, revision, scope/unidad/atributos | recurso | snapshot, target lock, refs, recarga | UPDATE parent + reemplazo valores | actor; ID/revisión no cero en Writer, positivos en app; dominio | tx + `FOR UPDATE`; recarga fuera | conflict y demás categorías | identidad almacenada se conserva; posible error post-commit al cambiar clase | `Writer.UpdateResource`; `Adapter.UpdateResource` |
| `DeactivateCatalog` | desactivar catálogo CAS | actor, kind, ID, revision | record confirmatorio | current/snapshot/recarga V2 + Get confirmatorio | UPDATE si cambia | actor/ID/revisión; catálogo válido | tx V2 si cambia; confirm-read fuera | categorías públicas | no-op omite CAS; confirm-read no atómico | `Writer.DeactivateCatalog`; bridge |
| `ReactivateCatalog` | reactivar catálogo CAS | actor, kind, ID, revision | record confirmatorio | current/snapshot/recarga V2 + Get confirmatorio | UPDATE si cambia | actor/ID/revisión; catálogo válido | tx V2 si cambia; confirm-read fuera | categorías públicas | no-op omite CAS | `Writer.ReactivateCatalog`; bridge |
| `DeactivateResource` | desactivar recurso CAS | actor, ID, revision | recurso | target locked + recarga | UPDATE si cambia | actor/ID/revisión; app exige ID positivo | tx + `FOR UPDATE` | conflict/not found/neutralizado | no-op compara revision en repo | `Writer.DeactivateResource`; bridge |
| `ReactivateResource` | reactivar recurso CAS | actor, ID, revision | recurso | GetByID, snapshot, target locked, recarga | UPDATE si cambia | actor/ID/revisión; identidad y refs activas | prelectura + tx con `FOR UPDATE` | conflict, identity, reactivation, neutralizado | activo actual retorna antes del CAS | `Writer.ReactivateResource`; bridge |
| `HardDeleteCatalog` | eliminar físicamente catálogo | actor, kind, ID, revision | error | current/dependencias/refs/snapshot/recarga | DELETE | actor/ID/revisión; inactivo y sin uso | mutex + tx V2 | conflict, invalid lifecycle, in use, neutralizado | CAS/FK protegen carrera; no confirm-read | `Writer.HardDeleteCatalog`; bridge |

## 31. Descriptores completos de los once kinds

### 31.1 Leyenda

`Mutable`, `ImmutableOnceReferenced` y `ImmutableAlways` son los tres valores posibles de `FieldDescriptor.Immutable`. En el registro actual sólo `code` usa `ImmutableOnceReferenced`; ningún campo usa `ImmutableAlways`. “No” en Guidance significa cadena vacía; `[]` significa slice vacío/nil efectivo en el descriptor.

| Kind | Campo | Kind de campo | Required | Immutable | Searchable | RefKind | RefScopedBy | AllowCreate | EnumValues | Guidance |
|---|---|---|---:|---|---:|---|---|---:|---|---|
| `CLASE` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `CLASE` | `name` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `CLASE` | `plural` | `FieldText` | sí | `Mutable` | no | no | `[]` | no | `[]` | no |
| `CLASE` | `slug` | `FieldText` | sí | `Mutable` | no | no | `[]` | no | `[]` | no |
| `CLASE` | `order` | `FieldInt` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `CLASE` | `aliases` | `FieldStringList` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `CLASE` | `keywords` | `FieldStringList` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `FAMILIA` | `class` | `FieldRef` | sí | `Mutable` | no | `CLASE` | `[]` | no | `[]` | no |
| `FAMILIA` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `FAMILIA` | `name` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `TIPO` | `class` | `FieldRef` | sí | `Mutable` | no | `CLASE` | `[]` | no | `[]` | no |
| `TIPO` | `family` | `FieldRef` | sí | `Mutable` | no | `FAMILIA` | `[class]` | no | `[]` | no |
| `TIPO` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `TIPO` | `name` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `CARACTERISTICA` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `CARACTERISTICA` | `name` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `CARACTERISTICA` | `valueType` | `FieldEnum` | sí | `Mutable` | no | no | `[]` | no | `CONTROLLED_OPTION=Opción controlada; INTEGER=Entero; DECIMAL=Decimal; QUANTITY=Cantidad; BOOLEAN=Booleano; CONTROLLED_TEXT=Texto controlado` | no |
| `CARACTERISTICA` | `dimension` | `FieldText` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `CARACTERISTICA` | `defaultIdentityParticipates` | `FieldBool` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `CONJUNTO_OPCIONES` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `CONJUNTO_OPCIONES` | `name` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `OPCION` | `optionSet` | `FieldRef` | sí | `Mutable` | no | `CONJUNTO_OPCIONES` | `[]` | sí | `[]` | no |
| `OPCION` | `characteristic` | `FieldRef` | sí | `Mutable` | no | `CARACTERISTICA` | `[]` | sí | `[]` | no |
| `OPCION` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `OPCION` | `label` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `RELACION_OPCIONES` | `optionSet` | `FieldRef` | sí | `Mutable` | no | `CONJUNTO_OPCIONES` | `[]` | no | `[]` | no |
| `RELACION_OPCIONES` | `fromCharacteristic` | `FieldRef` | sí | `Mutable` | no | `CARACTERISTICA` | `[]` | no | `[]` | no |
| `RELACION_OPCIONES` | `fromOption` | `FieldRef` | sí | `Mutable` | no | `OPCION` | `[optionSet, fromCharacteristic]` | no | `[]` | no |
| `RELACION_OPCIONES` | `toCharacteristic` | `FieldRef` | sí | `Mutable` | no | `CARACTERISTICA` | `[]` | no | `[]` | no |
| `RELACION_OPCIONES` | `toOption` | `FieldRef` | sí | `Mutable` | no | `OPCION` | `[optionSet, toCharacteristic]` | no | `[]` | no |
| `UNIDAD` | `code` | `FieldCode` | sí | `ImmutableOnceReferenced` | sí | no | `[]` | no | `[]` | no |
| `UNIDAD` | `name` | `FieldText` | sí | `Mutable` | sí | no | `[]` | no | `[]` | no |
| `UNIDAD` | `symbol` | `FieldText` | sí | `Mutable` | no | no | `[]` | no | `[]` | no |
| `UNIDAD` | `dimension` | `FieldText` | sí | `Mutable` | no | no | `[]` | no | `[]` | “Categoría de medición. Ejemplos: Longitud, Masa, Tiempo o Pieza.” |
| `POLITICA_UNIDAD` | `class` | `FieldRef` | sí | `Mutable` | no | `CLASE` | `[]` | no | `[]` | no |
| `POLITICA_UNIDAD` | `family` | `FieldRef` | sí | `Mutable` | no | `FAMILIA` | `[class]` | no | `[]` | no |
| `POLITICA_UNIDAD` | `unit` | `FieldRef` | sí | `Mutable` | no | `UNIDAD` | `[]` | sí | `[]` | no |
| `POLITICA_UNIDAD` | `allowed` | `FieldBool` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `POLITICA_UNIDAD` | `suggested` | `FieldBool` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `APLICABILIDAD` | `class` | `FieldRef` | sí | `Mutable` | no | `CLASE` | `[]` | no | `[]` | no |
| `APLICABILIDAD` | `family` | `FieldRef` | sí | `Mutable` | no | `FAMILIA` | `[class]` | no | `[]` | no |
| `APLICABILIDAD` | `type` | `FieldRef` | no | `Mutable` | no | `TIPO` | `[class, family]` | no | `[]` | no |
| `APLICABILIDAD` | `characteristic` | `FieldRef` | sí | `Mutable` | no | `CARACTERISTICA` | `[]` | sí | `[]` | no |
| `APLICABILIDAD` | `optionSet` | `FieldRef` | no | `Mutable` | no | `CONJUNTO_OPCIONES` | `[]` | sí | `[]` | no |
| `APLICABILIDAD` | `mode` | `FieldEnum` | sí | `Mutable` | no | no | `[]` | no | `REQUIRED=Requerido; OPTIONAL=Opcional; FORBIDDEN=Prohibido` | no |
| `APLICABILIDAD` | `identityParticipates` | `FieldBool` | no | `Mutable` | no | no | `[]` | no | `[]` | no |
| `PRESENTACION` | `class` | `FieldRef` | sí | `Mutable` | no | `CLASE` | `[]` | no | `[]` | no |
| `PRESENTACION` | `family` | `FieldRef` | sí | `Mutable` | no | `FAMILIA` | `[class]` | no | `[]` | no |
| `PRESENTACION` | `type` | `FieldRef` | sí | `Mutable` | no | `TIPO` | `[class, family]` | no | `[]` | no |
| `PRESENTACION` | `characteristic` | `FieldRef` | sí | `Mutable` | no | `CARACTERISTICA` | `[]` | no | `[]` | no |
| `PRESENTACION` | `position` | `FieldInt` | sí | `Mutable` | no | no | `[]` | no | `[]` | no |

### 31.2 Metadatos de kind, fuera de `FieldDescriptor`

| Kind | IdentityFields | ParentKind / ParentField | SoftDelete | Children declarados |
|---|---|---|---:|---|
| `CLASE` | `[code]` | no | sí | `FAMILIA(class)`, blocking |
| `FAMILIA` | `[class,code]` | `CLASE/class` | sí | `TIPO(family)`, `APLICABILIDAD(family)`, `POLITICA_UNIDAD(family)`, blocking |
| `TIPO` | `[class,family,code]` | `FAMILIA/family` | sí | `APLICABILIDAD(type)` blocking; `PRESENTACION(type)` no blocking |
| `CARACTERISTICA` | `[code]` | no | sí | `APLICABILIDAD(characteristic)`, `OPCION(characteristic)`, blocking |
| `CONJUNTO_OPCIONES` | `[code]` | no | sí | `OPCION(optionSet)`, `RELACION_OPCIONES(optionSet)`, blocking |
| `OPCION` | `[optionSet,characteristic,code]` | `CONJUNTO_OPCIONES/optionSet` | sí | `RELACION_OPCIONES(fromOption,toOption)`, blocking |
| `RELACION_OPCIONES` | `[optionSet,fromOption,toOption]` | `CONJUNTO_OPCIONES/optionSet` | sí | ninguno |
| `UNIDAD` | `[code]` | no | sí | `POLITICA_UNIDAD(unit)`, blocking |
| `POLITICA_UNIDAD` | `[class,family,unit]` | `FAMILIA/family` | sí | ninguno |
| `APLICABILIDAD` | `[class,family,type,characteristic]` | `FAMILIA/family` | sí | ninguno en descriptor; rules son parte del agregado |
| `PRESENTACION` | `[class,family,type,characteristic]` | `TIPO/type` | sí | ninguno |

## 32. Flujo detallado de errores y opacidad de frontera

### 32.1 Capas y conservación de causa

| Capa/tipo | ¿Implementa `Unwrap`? | Conservación interna | Riesgo de detalle SQL/pgx | Frontera que lo neutraliza |
|---|---:|---|---|---|
| sentinels `domain.Err*` | no; son valores de `errors.New` o wrappers estáticos | callers agregan contexto con `%w`; `errors.Is` conserva clasificación | no por sí mismos | `internal/core.Map` |
| `ErrIdentityConflict` | es un `fmt.Errorf` estático que envuelve `ErrResourceIntegrity` | sí, relación intencional de clasificación | no | `internal/core.Map`, con precedencia identity antes de integrity |
| `WrapReactivationImpossible(reason)` | el error retornado sí desenvuelve ambos `%w` | conserva outcome y causa domain/repository | sí, si `reason` conserva una causa técnica no clasificada | `internal/core.Map` elimina la causa |
| `WrapInvalidCatalog(reason)` | el error retornado sí desenvuelve ambos `%w` | conserva outcome y defectos unidos del catálogo | normalmente no SQL; puede envolver error interno | `internal/core.Map` |
| wrappers de aplicación/repositorio `fmt.Errorf("...: %w", err)` | sí | conservan causa para diagnóstico y `errors.Is/As` internos | sí: un error pgx no clasificado puede seguir en la cadena | bridge `mapError` -> `internal/core.Map` |
| `mapRepositoryError` | devuelve sentinel envuelto para 23505/23503/23514; otros retornan causa original | conserva sólo mensaje de constraint/PG dentro de la capa interna | sí internamente; 23503/23514 incorporan `pgErr.Message` | `internal/core.Map`; fallback seguro `INTERNAL` |
| `mapCatalogWriteError` / `mapCatalogDeleteError` | devuelve sentinels envueltos | conserva constraint/message como texto interno | sí internamente | `internal/core.Map` |
| `internal/core.Error` | **no** | no retiene causa; sólo código y mensaje seguro | no | ya es la frontera neutral |
| `resourcecore.Error` | **no** | no retiene causa; sólo código y mensaje | no | frontera pública final |

### 32.2 Resultado por código público

| Código/tipo | Fuentes internas principales | Wrapping interno y `errors.Is` | Posible fuga SQL/pgx antes del bridge | Neutralización y resultado público |
|---|---|---|---|---|
| `INVALID_ARGUMENT` | `core.ErrInvalidArgument`; validadores públicos producen directamente `resourcecore.Error` | aplicación puede devolver sentinel o wrapper; validación pública no guarda causa | no | `core.Map` o validación directa; mensaje seguro, sin `Unwrap` |
| `NOT_FOUND` | `ErrResourceNotFound`, `ErrCatalogRecordNotFound` | repositorios suelen usar `%w`; servicios a veces devuelven sentinel limpio | no, salvo error de lectura no clasificado que no se convierte en not-found | `core.Map` reconoce sentinels y descarta cadena |
| `DUPLICATE` | `ErrDuplicateResource`, `ErrCatalogDuplicate` | SQLSTATE 23505 se traduce a sentinel envuelto con constraint interno | sí internamente: nombre de constraint | `core.Map` devuelve “the record already exists” |
| `INVALID_REFERENCE` | `ErrResourceReference`, `ErrCatalogReference` | validación y SQLSTATE 23503/23514/23502 se envuelven | sí internamente: `pgErr.Message` | `core.Map` devuelve mensaje genérico de referencia |
| `VALIDATION` | `ErrResourceValidation` y wrappers de dominio | `validation(...)` conserva sentinel con `%w`; `errors.Join` puede participar dentro de catálogo | normalmente no; texto de regla queda interno | `core.Map` devuelve mensaje genérico de validación |
| `INTEGRITY` | `ErrResourceIntegrity` | cardinalidad/codec se envuelven con `%w`; se evalúa después de identity | sí internamente si surge de driver no clasificado junto al wrapper | `core.Map` descarta detalle y causa |
| `IDENTITY_CONFLICT` | `ErrIdentityConflict` | el sentinel estático también satisface `ErrResourceIntegrity`; precedencia evita degradación | no SQL directo | `core.Map` selecciona identity primero y crea error opaco |
| `INVALID_LIFECYCLE` | `ErrInvalidLifecycle` | normalmente sentinel directo | no | `core.Map` produce mensaje seguro |
| `REACTIVATION_IMPOSSIBLE` | `WrapReactivationImpossible` | dos `%w`: outcome y razón subyacente siguen consultables internamente | posible si razón es error de repositorio no clasificado | `core.Map` selecciona outcome antes de categorías amplias y pierde causa |
| `INVALID_CATALOG` | `WrapInvalidCatalog` | preserva `ErrInvalidCatalog` y el error de `Validate`, incluso `errors.Join` | no normalmente | `core.Map` retiene sólo categoría/mensaje seguro |
| `IN_USE` | `ErrCatalogInUse` | guard de servicio retorna sentinel; SQLSTATE 23503 de delete lo envuelve | sí internamente en backstop FK | `core.Map` devuelve mensaje genérico de uso |
| `IMMUTABLE_CODE` | `ErrCodeImmutable` | sentinel directo antes del UPDATE | no | `core.Map` devuelve mensaje seguro |
| `CONFLICT` | `ErrRevisionConflict` alias de `ErrResourceRevisionConflict` | repositorios/servicios pueden devolver directo o envuelto; clasificación exacta por `errors.Is` | no; se decide por revisión/filas, no por texto SQL | `core.Map` lo evalúa primero y elimina causa |
| `UNAVAILABLE` | `core.ErrUnavailable`, cancelación, deadline, repo V2 ausente, latch | errores de catálogo envuelven `core.ErrUnavailable`; context errors se reconocen | posible causa técnica interna en fallo indeterminado, registrada con `core.Record` | `core.Map` devuelve mensaje temporal genérico |
| `INTERNAL` | cualquier error no clasificado | puede conservar toda la cadena `%w` dentro de Core | **sí internamente**, incluido pgx/SQL no mapeado | fallback de `core.Map` produce sólo “an internal error occurred”; no `Unwrap` público |

### 32.3 Recorrido concreto

```text
PostgreSQL/pgx
  -> repositorio: traduce SQLSTATE conocido o conserva error técnico con %w
  -> aplicación: reconoce sentinels o agrega contexto con %w
  -> bridge mapError: internal/core.Map(errors.Is/errors.As, precedencia fija)
  -> internal/core.Error sin causa
  -> resourcecore.NewError(código, mensaje seguro)
  -> resourcecore.Error sin Unwrap
```

**Confirmado.** Las causas SQL/pgx pueden permanecer disponibles dentro de la cadena interna para diagnóstico y `core.Record`; no cruzan el bridge. Ni `internal/core.Error` ni `resourcecore.Error` exponen `Unwrap`, y los mensajes públicos son los quince textos fijos de `internal/core.errors.messages`.
