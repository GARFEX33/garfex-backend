# Auditoría del Core GARFEX previa a Pi

> Auditoría **READ-ONLY** del estado local observado el 18 de agosto de 2026. Único artefacto producido: este documento. No se ejecutaron builds, tests, migraciones ni conexiones a PostgreSQL.

## Convenciones de evidencia

- **[HECHO]** comprobado en código, SQL, configuración o pruebas actuales; se cita ruta y símbolo.
- **[RECOMENDACIÓN]** juicio de arquitectura; no describe comportamiento existente.
- **[NO VERIFICADO]** evidencia insuficiente en este repositorio. La expresión literal `no verificado` se conserva para facilitar búsquedas.
- Las categorías usadas al final son: **A** conservar tal cual; **B** conservar adaptando el límite; **C** reutilizar como dependencia/servicio compartido; **D** candidato a reemplazo por Pi en la capa exterior; **E** retirar sólo con evidencia concreta.

### Cobertura y método

**[HECHO]** Se confirmó como raíz `/home/garfex/PROGRAMACION/garfex-costos-unitarios-workspace`; `.codegraph/` estaba presente. CodeGraph fue la fuente primaria para símbolos, callers, call paths e impacto. Luego se inventariaron archivos y se inspeccionaron SQL, configuración, documentación y scripts no cubiertos por el grafo.

**[HECHO]** El inventario Git contiene 175 archivos versionados: 135 archivos Go, 75 archivos `*_test.go`, 14 migraciones SQL (siete pares up/down) y 452 funciones `Test*`. Se cubrieron sistemáticamente `cmd`, `internal/config`, `internal/domain`, `internal/app`, `internal/postgres`, `internal/tui`, `internal/modules/suppliers`, `migrations`, scripts de DB, Compose, CI, documentación de arquitectura y el cambio OpenSpec vigente. No se leyó `.env` para evitar exponer secretos.

---

## 1. Resumen ejecutivo

**[HECHO]** GARFEX ya no es sólo una TUI o una “Fase 0”. Existe un Core funcional de Recursos Maestros con dominio, casos de uso, puertos, persistencia PostgreSQL, administración de catálogo y delivery Bubble Tea. El arranque productivo respeta el flujo `TUI -> aplicación -> puerto de dominio -> PostgreSQL`; la composición está concentrada en `cmd/garfex/main.go:run`.

**[HECHO]** El Core reutilizable más maduro es Resource Master:

1. `internal/domain` contiene catálogo, canonicalización, identidad v1, reglas, presentación, lifecycle y contratos de repositorio.
2. `internal/app/recursos.Service` contiene los casos de uso de instancias.
3. `internal/app/catalogo.Service` administra once clases de estructura catalográfica y publica snapshots coherentes mediante `domain.CatalogAuthority`.
4. `internal/postgres` carga el catálogo y persiste instancias/estructura con transacciones y errores de dominio.
5. `internal/tui` consume interfaces mínimas de aplicación; no importa `internal/postgres` ni ejecuta SQL.

**[HECHO]** Supplier Master también existe como Core independiente en `internal/modules/suppliers/**`, con dominio, servicio, puerto y adaptador PostgreSQL, respaldado por `migrations/000006_supplier_master.up.sql`. Sin embargo, `cmd/garfex/main.go:run` no construye `suppliers.New`, y no hay delivery TUI/API/MCP productivo que lo alcance.

**[HECHO]** No existe una implementación real de IA: no hay SDK ni cliente LLM, provider, streaming, prompt runtime, tool runtime, router de intenciones, MCP de producto ni persistencia de conversaciones. Sí existe un protocolo determinista de interacción en `internal/tui/interaction.go`, adaptadores de capacidades cerradas y un `AssistantShellAgent` explícitamente placeholder.

**[HECHO]** Pi puede reemplazar partes de UX/orquestación, pero no hay evidencia para que reemplace dominio, casos de uso, persistencia, migraciones, configuración ni políticas de negocio. Las capacidades exactas, lenguaje, proceso y modelo de extensiones de Pi son **no verificado** en este repositorio.

**[RECOMENDACIÓN]** La frontera segura es hacer que Pi consuma el Core, no que lo replique. Para un primer vertical sólo lectura, Pi debería llamar adaptadores delgados sobre `catalogo.Service.List`, `recursos.Service.SearchPage`, `recursos.Service.Get` y `recursos.Service.Describe`; ninguna regla debe trasladarse a prompts o componentes de Pi.

**[HECHO]** Hay deuda documental comprobada: `README.md:3` afirma que no hay esquema de negocio ni servicios de aplicación, lo cual contradice el código y las migraciones actuales. `openspec/changes/resource-master-technical-debt/tasks.md:44-47` deja sin marcar lifecycle, hidratación set-based, paginación y documentación, aunque esos símbolos ya existen; el artefacto OpenSpec no puede tratarse como estado operativo actual.

---

## 2. Arquitectura actual

### 2.1 Mapa de capas y responsabilidades

| Capa | Responsabilidad comprobada | Callers / dependencias | Independencia de TUI |
|---|---|---|---|
| `cmd/garfex` | Entrypoint, comandos directos, conexión, carga y composición | SO -> `main` -> `run`; depende de config, app, domain, postgres y tui | No aplica: composition root |
| `internal/config` | Validación de entorno, secreto redactado y DSN | `cmd/garfex.run`, `tui.Config`; sólo stdlib | Sí |
| `internal/domain` | Entidades, reglas, catálogos, identidad, lifecycle, puertos y errores | app, postgres y TUI usan sus tipos | Sí; no importa Bubble Tea ni pgx |
| `internal/app/recursos` | Casos de uso de instancias Resource | TUI y composition root; depende de `domain.ResourceRepository` | Sí |
| `internal/app/catalogo` | Casos de uso de estructura del catálogo, validación y publicación de snapshot | TUI y composition root; depende de `domain.CatalogAdminRepository` | Sí |
| `internal/postgres` | Implementaciones Resource/Catalog, SQL, transacciones, loaders y codecs | sólo composition root y tests lo construyen | Sí respecto de TUI; acoplado a pgx/PostgreSQL |
| `internal/modules/suppliers` | Bounded context Supplier con domain/app/postgres/module | hoy sólo sus tests; `module.New` no está compuesto | Sí |
| `internal/tui` | Modelo Bubble Tea, navegación, protocolo de interacción y adaptadores delivery | construido por `cmd/garfex.run` | No; es delivery |
| `migrations`, `scripts/db` | Esquema, grants, seeds, roles y ejecución de migraciones | operación/CI | Sí |

### 2.2 Flujo real de arranque

```text
SO
 -> cmd/garfex.main
 -> run(args, os.LookupEnv, ..., newPostgresInfra, postgres.LoadResourceCatalog)
    ├─ args == ["version"] -> imprime version
    ├─ args == ["config", "check"] -> config.Load -> resumen redactado
    └─ sin args
       -> config.Load -> Config.DSN
       -> newPostgresInfra
          -> pgxpool.New -> Pool.Ping
          -> postgres.NewResourceRepository(pool)
       -> postgres.LoadResourceCatalog(pool)
          -> transacción RepeatableRead + ReadOnly
          -> loadClasses/OptionSets/Families/Types/PresentationFields/Units/
             UnitPolicies/Definitions/AttributeRules/Attributes/Options/Relations
       -> ResourceCatalog.Validate
       -> domain.NewCatalogAuthority(catalog)
       -> recursos.NewServiceWithCatalogAuthority(resourceRepo, authority)
       -> catalogo.NewServiceWithCatalogAuthority(
            postgres.NewCatalogAdminRepository(pool), registry, authority)
       -> tui.NewResourcesWorkspaceAdapter(...) por clase activa y global
       -> tui.NewCatalogAdminAdapter(...)
       -> tui.NewAssistantShellAgent()
       -> tui.NewWithCatalogAuthority(...)
       -> tea.NewProgram(model).Run()
```

Referencias: `cmd/garfex/main.go:main`, `run`, `newPostgresInfra`; `internal/postgres/catalog_loader.go:LoadResourceCatalog`; `internal/tui/model.go:NewWithCatalogAuthority`.

**[HECHO]** El arranque falla cerrado ante configuración inválida, DB inaccesible, catálogo vacío o `ResourceCatalog.Validate` fallido (`cmd/garfex/main.go:62-108`). El pool no se cierra explícitamente tras una salida normal de la TUI; su liberación queda al terminar el proceso.

### 2.3 Dirección de dependencias y entrypoints

**[HECHO]** Los entrypoints de negocio son razonablemente limpios: TUI declara interfaces pequeñas (`resourceSearcher`, `resourceGetter`, `resourceCreator`, `resourceUpdater`, `resourceLifecycle` y equivalentes de catálogo) y `*recursos.Service` / `*catalogo.Service` las satisfacen estructuralmente. No se detectó dominio dependiente de TUI, servicio dependiente de chat, TUI importando pgx ni SQL productivo fuera de adaptadores PostgreSQL.

**[HECHO]** `cmd/garfex` sí conoce `*pgxpool.Pool` en sus seams `infraBuilder` y `catalogLoader`. Es aceptable en composition root, pero esos contratos no son neutrales para otro delivery.

**[HECHO]** Todos los paquetes están bajo `internal/`. Por reglas de Go, un proyecto Pi en otro módulo/repository no podría importarlos directamente salvo que esté dentro del árbol autorizado por el import path padre. Éste es el principal bloqueo técnico para “usar el Core como librería” sin mover/publicar contratos.

---

## 3. Inventario del Core

### 3.1 Recursos Maestros

| Componente | Responsabilidad y lógica | Callers reales |
|---|---|---|
| `domain.ResourceCatalog` | Snapshot de clases, familias, tipos, unidades, políticas, definiciones, bindings, opciones, relaciones, reglas y presentación | loader, authority, app y TUI |
| `domain.NewResource` | Canonicaliza, valida scope/unidad/atributos/reglas/relaciones/actividad y deriva identidad | `recursos.Service.Create/Update/Reactivate` |
| `domain.Resource` | Agregado con ID persistente, identidad natural v1, atributos y estado | app, repositorio, TUI |
| `domain.ResourceRepository` | Puerto Create/Get/Search/Update/Deactivate/Reactivate | `recursos.Service`; implementa `postgres.resourceRepository` |
| `recursos.Service` | Get, Search, SearchPage, Describe, Create, Update, Deactivate, Reactivate | `ResourcesWorkspaceAdapter`, handler y composition root |
| `domain.CatalogRegistry` | Descriptores fijos de 11 tipos administrables | servicio y motor TUI genérico |
| `catalogo.Service` | List/Get/Create/Update/lifecycle/delete con validación de snapshot | `CatalogAdminAdapter` |
| `postgres.LoadResourceCatalog` | Hidratación consistente y completa desde 11+ tablas | sólo arranque y tests |
| `postgres.resourceRepository` | CRUD, lifecycle, búsqueda paginada y atributo efectivo | `recursos.Service` |
| `postgres.catalogAdminRepository` | CRUD genérico por kind, dependencias y referencias | `catalogo.Service` |

### 3.2 Supplier Master

**[HECHO]** `suppliers/domain` modela `Supplier`, `Branch` y `Contact`; los hijos siempre llevan `SupplierID`, y `Contact.BranchID` es opcional pero la FK compuesta garantiza que la sucursal pertenezca al mismo proveedor (`migrations/000006_supplier_master.up.sql:45-64`).

**[HECHO]** `suppliers/app.Service` expone casos de uso separados en `supplier.go`, `branch.go` y `contact.go`. El puerto `domain.Repository` enumera 15 operaciones de create/get/list-or-search/update/lifecycle. `suppliers/postgres` implementa SQL y traduce `23505`, `23503`, `23502` y `23514` a errores de dominio (`postgres/repository.go:mapWriteError`).

**[HECHO]** `suppliers.Module` expone tanto `Service` como `Repository`. El servicio es el entrypoint correcto; exponer el repository facilita un bypass futuro, aunque no hay caller productivo actual que lo haga. No existe integración con Resource Master, precios o productos de proveedor en el código observado.

### 3.3 Configuración, plataforma y operación

- **[HECHO]** `config.Load` requiere seis variables DB y valida puerto, SSL mode y log level; `Secret` redacta String/GoString/text/JSON, mientras `Reveal` queda como escape explícito para `Config.DSN` (`internal/config/config.go`).
- **[HECHO]** PostgreSQL 17.5 corre en Compose; local publica sólo `127.0.0.1:5432`, integración no publica puerto (`compose*.yaml`).
- **[HECHO]** `garfex_admin` posee esquema y migra; `garfex_app` es runtime sin superuser/createdb/createrole (`scripts/db/init/00-roles.sh`).
- **[HECHO]** CI aplica format, vet, golangci-lint, race tests, build Go, imagen, usuario 65532 y smoke `version` (`.github/workflows/ci.yml`).

### 3.4 Lo que no existe como Core actual

**[HECHO]** No se encontraron módulos de APU, cotizaciones, compras, documentos, pricing, usuarios, permisos, auditoría de eventos o runtime agent en código Go productivo. Aparecen como conceptos en documentación histórica/futura, no como implementaciones actuales. Su existencia operativa es **no verificado**.

---

## 4. Auditoría profunda de Recursos Maestros

### 4.1 Entidades, IDs y relaciones

| Concepto | Tipo/tabla | Identidad y relación comprobada |
|---|---|---|
| Clase | `ResourceClass` / `resource_classes` | DB `BIGSERIAL`; código único, slug único; código es join key y nombre/plural/slug son presentación |
| Familia | `ResourceFamily` / `resource_families` | DB ID; natural `(class, code)`; FK compuesta preserva clase |
| Tipo | `ResourceType` / `resource_types` | DB ID; natural `(family, code)`; FK compuesta preserva familia/clase |
| Unidad | `UnitDefinition` / `unit_definitions` | DB ID; código y símbolo únicos; nombre requerido desde 000004 |
| Política de unidad | `ResourceUnitPolicy` / `resource_unit_policies` | PK `(family_id, unit_id)`; `suggested => allowed` |
| Característica | `AttributeDefinition` / `attribute_definitions` | DB ID; código global único; tipo de valor y dimensión |
| Aplicabilidad | `ResourceAttribute` / `resource_attributes` | binding por familia o tipo; índices parciales evitan duplicado por scope |
| Regla | `AttributeRule` / `resource_attribute_rules` | hija ordenada del binding; modo, identidad y N/A condicionales |
| Conjunto | `ResourceOptionSet` / `resource_option_sets` | código PK; permite aislar vocabularios |
| Opción | `AttributeOption` / `attribute_options` | PK `(option_set, definition, code)` |
| Relación | `AttributeOptionRelation` / tabla homónima | pares válidos entre opciones del mismo set |
| Presentación | `PresentationField` / `resource_type_presentation_fields` | orden único por tipo |
| Recurso | `Resource` / `recursos` | DB ID estable + unique `(class_id, identity_key)`; soft lifecycle |
| Valor | `ResourceAttributeValue` / `resource_attribute_values` | uno por `(resource, binding)`; payload union tipado |

Referencias: `internal/domain/resource_types.go`, `resource_class.go`, `catalog_record.go`; `migrations/000002_resource_master.up.sql`, 000003 y 000004.

### 4.2 Canonicalización, identidad y presentación

**[HECHO]** `domain.NewResource` canonicaliza scope y unidad; normaliza códigos de atributo, rechaza repetidos/desconocidos, canonicaliza payload por definición, aplica reglas y valida relaciones. Ordena los códigos antes de producir el resultado (`internal/domain/resource_validation.go:NewResource`, `newResource`).

**[HECHO]** `IdentityKey` es determinista y delimitada por longitud en bytes:

```text
v1|
  len:CLASS
  len:FAMILY
  len:TYPE
  [len:attribute-code][len:value-type][len:canonical-value]...
```

Sólo participan atributos efectivos con `IdentityParticipates`; la unidad natural, el nombre de presentación y atributos no identitarios no participan (`resource_validation.go:52-97`). La restricción `identity_key LIKE 'v1|%'` se agrega en 000007 después del mapa/auditoría de 000005.

**[HECHO]** `ResourceCatalog.Describe` resuelve el nombre del tipo y concatena sólo `PresentationFields` ordenados, omitiendo ausentes y `NOT_APPLICABLE`. Si no hay campos devuelve sólo el nombre del tipo; si no resuelve el tipo devuelve cadena vacía (`resource_presentation.go:Describe`).

**[HECHO]** Existe además presentación técnica en `tui.renderResourceDetail` y helpers de editor. Es delivery, no identidad. Parte de la presentación de labels/opciones se resuelve en TUI contra el snapshot (`resourceAttributePresentation`), por lo que conviene no reutilizar esos helpers fuera de TUI.

### 4.3 Lifecycle y restricciones activas

**[HECHO]** Creación, update y reactivación exigen cadena activa: clase, familia, tipo, unidad, política, binding, definición, option set, regla, opción y relación aplicables (`ResourceCatalog.validateActiveChain`). Lecturas históricas se hidratan sin exigir actividad mediante `HydrateResource`/`RehydrateResource`.

**[HECHO]** `recursos.Service.Deactivate(id)` delega al repositorio; no borra. `Reactivate(id)` carga por ID, reconstruye contra el catálogo actual, compara identidad y pide una transición transaccional con identity esperada. Es idempotente si ya está activo (`service.go:138-208`).

**[HECHO]** Search por defecto normaliza a activos; inactivos requieren `LifecycleScopeInactive`. `SearchCriteria.Normalize` limita la página a máximo 50 y rechaza límites inválidos; PostgreSQL solicita `limit+1`, ordena por `identity_key, id` y calcula `HasNext/HasPrevious` (`resource_repository_search.go:SearchPage`).

### 4.4 Operaciones reales, contratos y errores

| Operación | Firma real | Validación / I/O / error relevante |
|---|---|---|
| Get | `Service.Get(ctx, classCode, identityKey) (Resource, error)` | argumentos no vacíos; `ErrResourceNotFound`; repository incluye inactivos |
| Search | `Service.Search(ctx, SearchCriteria) ([]Resource, error)` | wrapper collection; sin metadatos de página |
| SearchPage | `Service.SearchPage(ctx, SearchCriteria) (ResourcePage, error)` | normaliza; exige repo con `ResourcePageRepository` |
| Describe | `Service.Describe(Resource) string` | snapshot actual de `CatalogAuthority`; sin I/O |
| Create | `Service.Create(ctx, CreateCommand) (Resource, error)` | `NewResource` antes del repo; duplicate/reference/integrity tipados |
| Update | `Service.Update(ctx, UpdateCommand) (Resource, error)` | ID positivo; reconstruye identidad; preserva sólo ID estable |
| Deactivate | `Service.Deactivate(ctx, id) (LifecycleResult, error)` | ID positivo; soft transition |
| Reactivate | `Service.Reactivate(ctx, id) (LifecycleResult, error)` | carga, revalida catálogo/identidad y transición guardada |

**[HECHO]** El alias `Service.Delete` sólo llama `Deactivate`; no elimina físicamente (`internal/app/recursos/service.go:153-160`). Un nuevo adaptador no debería publicarlo como delete.

#### Flujos textuales completos de instancias Resource Master

**[HECHO]** Resource Master no publica un método `List` separado: el listado paginado es una búsqueda con `SearchCriteria`, y la TUI productiva exige la variante `SearchPage`. Un texto vacío puede representar un listado dentro del contrato de aplicación/repositorio; la búsqueda interactiva ordinaria llega como `InputText`.

```text
LISTAR / BUSCAR
Model.respond(InteractionInput{Kind: InputText, Value: texto})
 -> ResourcesWorkspaceAdapter.Respond
 -> searchResponse(ctx, SearchCriteria{
      Text: texto, ClassCode: classFilter,
      LifecycleScope: LifecycleScopeActive, Limit: 10,
    })
 -> searchPage
    -> type assertion a.resources.(interface { SearchPage(...) })
 -> recursos.Service.SearchPage(ctx, criteria)
    -> SearchCriteria.Normalize()
    -> domain.ResourcePageRepository.SearchPage(ctx, normalized)
 -> postgres.resourceRepository.SearchPage
    -> SELECT parametrizado sobre recursos/clase/familia/tipo/unidad
    -> loadEffectiveAttributes(ctx, resourceIDs)
    -> domain.HydrateResource por fila
 -> ResourcePage{Resources, Criteria, HasPrevious, HasNext}
 -> ResourcesWorkspaceAdapter.renderSearchPage
    -> describer.Describe(resource) para cada opción canónica
```

```text
DETALLE
selección QuestionRequest{Key: "resources-search-results",
                          Value: classCode + "|" + identityKey}
 -> ResourcesWorkspaceAdapter.Respond
 -> detailResponse(ctx, value)
    -> strings.Cut(value, "|")
    -> resourcesGetter.Get(ctx, classCode, identityKey)
 -> recursos.Service.Get
    -> domain.ResourceRepository.Get
 -> postgres.resourceRepository.Get
    -> fila base + loadEffectiveAttributes
    -> domain.HydrateResource
 -> ResourcesWorkspaceAdapter.resourcePresentation
    -> describer.Describe -> recursos.Service.Describe
    -> ResourceCatalog.Describe
 -> StructuredResult + detailActionsRequest(Edit/Duplicate/Lifecycle/Back)
```

```text
CREAR
Action{ID: "create-resource"}
 -> ResourcesWorkspaceAdapter.Respond
 -> startCreateEditor
    -> classQuestion (sólo workspace global) / familyQuestion
    -> typeQuestion
    -> attributePickerQuestion + preguntas tipadas por atributo
    -> unitQuestion
    -> confirmación
 -> answerEditConfirmation
 -> finishEditor(ctx, unit)
    -> persistenceValues + domainScope
    -> creator.Create(ctx, domain.CreateCommand{Scope, NaturalUnit, Attributes})
 -> recursos.Service.Create
    -> CatalogAuthority.Current
    -> domain.NewResource (canonicalización, reglas, relaciones, identidad v1)
    -> domain.ResourceRepository.Create
 -> postgres.resourceRepository.Create
    -> Resource.ValidateForPersistence
    -> BEGIN -> INSERT recursos -> persistAttributeValue por valor
    -> verifyAttributeCount -> COMMIT/rollback
 -> Resource canónico o error tipado
 -> StructuredResult; ante duplicado, Get abre el recurso existente
```

```text
EDITAR
detailActionsRequest -> Action{ID: editActionID}
 -> ResourcesWorkspaceAdapter.Respond
 -> startEditEditor
    -> toma lastDetail y bloquea una clase conocida inactiva
    -> resourceEditorState{mode: editorModeEdit,
         originalID, scope, valores y unidad actuales}
    -> attributePickerQuestion -> cambios -> unitQuestion -> confirmación
 -> answerEditConfirmation
 -> finishEditor(ctx, unit)
    -> updater.Update(ctx, domain.UpdateCommand{
         ID: originalID, Scope, NaturalUnit, Attributes,
       })
 -> recursos.Service.Update
    -> valida ID > 0
    -> domain.NewResource vuelve a derivar identidad; conserva sólo el ID
    -> domain.ResourceRepository.Update
 -> postgres.resourceRepository.Update
    -> BEGIN -> resolver referencias activas -> UPDATE recursos
    -> DELETE valores anteriores -> persistAttributeValue
    -> verifyAttributeCount -> COMMIT/rollback
```

```text
DESACTIVAR
detailActionsRequest -> Action{ID: deactivateActionID}
 -> startLifecycleConfirmation(false)
 -> ConfirmationRequest{Key: resourcesLifecycleConfirmKey}
 -> answerLifecycleConfirmation(ctx, "yes")
 -> resourceLifecycle.Deactivate(ctx, lastDetail.ID)
 -> recursos.Service.Deactivate(ctx, id)
    -> valida id > 0
    -> domain.ResourceRepository.Deactivate
 -> postgres.resourceRepository.Deactivate
 -> setLifecycle(ctx, id, false, "")
    -> BEGIN -> SELECT ... FOR UPDATE
    -> no-op idempotente si ya estaba inactivo
    -> UPDATE recursos SET active=false -> COMMIT
    -> Get para devolver LifecycleResult{Resource, Changed}
 -> actualiza lastDetail y vuelve al menú de detalle
```

```text
REACTIVAR
detailActionsRequest -> Action{ID: reactivateActionID}
 -> startLifecycleConfirmation(true)
 -> ConfirmationRequest -> answerLifecycleConfirmation(ctx, "yes")
 -> resourceLifecycle.Reactivate(ctx, lastDetail.ID)
 -> recursos.Service.Reactivate(ctx, id)
    -> valida id > 0
    -> repo.GetByID -> postgres.GetByID -> Get
    -> no-op idempotente si ya estaba activo
    -> CatalogAuthority.Current
    -> domain.NewResource con catálogo activo actual
    -> compara candidate.IdentityKey con la identidad almacenada
    -> domain.ResourceRepository.Reactivate(ctx, id, candidate.IdentityKey)
 -> postgres.resourceRepository.Reactivate
 -> setLifecycle(ctx, id, true, expectedIdentityKey)
    -> BEGIN -> SELECT ... FOR UPDATE
    -> verifica identidad, referencias activas y conflicto de duplicado
    -> UPDATE recursos SET active=true -> COMMIT -> Get
 -> LifecycleResult o ErrResourceNotFound/ErrDuplicateResource/
    ErrResourceReference/ErrResourceIntegrity
```

Referencias: `internal/tui/resources_workspace_dispatch.go:ResourcesWorkspaceAdapter.Respond`, `searchResponse`, `searchPage`, `detailResponse`, `startLifecycleConfirmation`, `answerLifecycleConfirmation`; `internal/tui/resource_editor.go:startCreateEditor`, `startEditEditor`, `finishEditor`; `internal/app/recursos/service.go`; `internal/postgres/resource_repository_crud.go:setLifecycle`; `internal/postgres/resource_repository_search.go:SearchPage`.

### 4.5 Persistencia PostgreSQL

#### Create/update

```text
ResourcesWorkspaceAdapter.finishEditor
 -> recursos.Service.Create/Update
 -> domain.NewResource
 -> domain.ResourceRepository.Create/Update
 -> postgres.resourceRepository
 -> BEGIN
 -> resolver clase/familia/tipo/unidad/política activa
 -> INSERT/UPDATE recursos
 -> resolver exactamente un binding por valor
 -> INSERT resource_attribute_values
 -> verificar cardinalidad
 -> COMMIT (o rollback diferido)
```

**[HECHO]** `Create` y `Update` llaman `Resource.ValidateForPersistence`, usan transacción y fallan ante referencias inactivas o cardinalidad inconsistente (`resource_repository_crud.go`). `Update` elimina y reconstruye valores dentro de la misma transacción.

#### Get/search

```text
Get(class, identity)
 -> fila base de recursos + joins de scope
 -> loadEffectiveAttributes(resource IDs)
 -> HydrateResource(snapshot)

SearchPage(criteria)
 -> criteria.Normalize
 -> query base limitada y filtros parametrizados
 -> detección de ganador efectivo ambiguo
 -> UNA carga set-based de atributos para todos los IDs
 -> HydrateResource por fila -> ResourcePage
```

**[HECHO]** Los filtros de texto usan parámetros y `ILIKE`; filtros de atributos se codifican por tipo. No se encontró interpolación de input de usuario en nombres de tabla en el repositorio de recursos. El repositorio de catálogo usa tablas/columnas de un mapa estático de kinds, no nombres recibidos libremente.

### 4.6 Carga de clases, familias, tipos, características y unidades

```text
cmd.run
 -> postgres.LoadResourceCatalog (RepeatableRead, ReadOnly)
    -> loadClasses                 -> ResourceCatalog.Classes
    -> loadOptionSets              -> OptionSets
    -> loadFamilies                -> Families
    -> loadTypes                   -> Types
    -> loadPresentationFields      -> PresentationFields
    -> loadUnits                   -> Units
    -> loadUnitPolicies            -> UnitPolicies
    -> loadDefinitions             -> Definitions/"características"
    -> loadAttributeRules          -> rules por resource_attribute_id
    -> loadAttributes(rules)       -> Attributes
    -> loadOptions                 -> Options
    -> loadRelations               -> Relations
 -> catalog.Validate
 -> CatalogAuthority
```

**[HECHO]** El loader preserva `active` de las estructuras agregadas por 000003, incluyendo OptionSet, definiciones, bindings, reglas, relaciones, políticas y campos de presentación. Esto verifica y supera la brecha histórica de `active` que sólo servía para orientar la búsqueda.

**[HECHO]** `SeedResourceCatalog` conserva una fixture Go equivalente al seed base para tests/desarrollo: 3 clases, 2 familias, 2 tipos, 2 unidades, 8 definiciones, opciones, relaciones y presentación (`internal/domain/resource_catalog.go`). PostgreSQL es la fuente de estructura en runtime; la paridad total de datos luego de modificaciones administrativas o migraciones futuras es **no verificado** sin ejecutar una base real.

### 4.7 Administración de estructura

**[HECHO]** `CatalogRegistry` registra 11 kinds: `CLASE`, `FAMILIA`, `TIPO`, `CARACTERISTICA`, `CONJUNTO_OPCIONES`, `OPCION`, `RELACION_OPCIONES`, `UNIDAD`, `POLITICA_UNIDAD`, `APLICABILIDAD` y `PRESENTACION` (`catalog_kind.go:11-23`).

**[HECHO]** `catalogo.Service.Create/Update` aplica una mutación sobre copia del snapshot, ejecuta `Validate`, persiste y sólo entonces publica el nuevo snapshot/versión. `Update` protege códigos `ImmutableOnceReferenced` consultando `ReferencedByResources`. Delete consulta dependencias desde el adaptador/servicio y el repository traduce restricciones.

#### Surface real de `catalogo.Service`

**[HECHO]** `Service` depende de `domain.CatalogAdminRepository`, `domain.CatalogRegistry`, un `domain.ResourceCatalog` en memoria y `*domain.CatalogAuthority`; `sync.Mutex` serializa sus mutaciones. En producción, `cmd/garfex/main.go:run` inyecta `postgres.NewCatalogAdminRepository(pool)`, por lo que cada llamada indicada abajo termina en los handlers estáticos de `internal/postgres/catalog_admin_kinds.go` y las tablas catalogográficas reales.

| Caso de uso y firma exacta | Input / output | Validación y errores | Dependencia y PostgreSQL |
|---|---|---|---|
| `List(ctx context.Context, kind domain.CatalogKindCode, filter domain.CatalogFilter) ([]domain.CatalogRecord, error)` | Kind + texto, parent refs, status, limit y offset; devuelve filas con ID de DB | El servicio delega sin validación propia. El repository rechaza kind no registrado con `ErrCatalogKindUnknown`; `CatalogFilter.Status` falla cerrado a activos en los builders | `repo.List` -> `catalogAdminRepository.List` -> `kindTables[kind].list`; SELECT parametrizado y joins específicos por kind |
| `Get(ctx context.Context, kind domain.CatalogKindCode, id int64) (domain.CatalogRecord, error)` | Kind + ID persistente; devuelve registro genérico tipado | Sólo `id == 0` produce `catalogo.ErrInvalidArgument`; negativos se delegan. Kind desconocido -> `ErrCatalogKindUnknown`; fila ausente -> `ErrCatalogRecordNotFound` desde el adaptador | `repo.Get` -> `catalogAdminRepository.Get` -> handler `get` y SELECT por ID/clave compuesta |
| `Create(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) (domain.CatalogRecord, error)` | Intent genérico; devuelve el mismo registro con `Kind` impuesto e `ID` asignado | Bajo mutex: `ApplyCatalogMutation(OpInsert)` sobre copia -> `ResourceCatalog.Validate`; ningún write si falla. Persistencia puede devolver `ErrCatalogDuplicate` (SQLSTATE 23505), `ErrCatalogReference` (23503/23514/23502), kind desconocido u otro error | `repo.Insert` -> `catalogAdminRepository.Insert`; transacción, handler `insert`, COMMIT; luego `publish(next)` |
| `Update(ctx context.Context, kind domain.CatalogKindCode, rec domain.CatalogRecord) (domain.CatalogRecord, error)` | Requiere `rec.ID`; devuelve el registro actualizado | `rec.ID == 0` -> `ErrInvalidArgument`; carga current; kind debe existir en registry; código `ImmutableOnceReferenced` cambiado consulta `ReferencedByResources` y puede devolver `ErrCodeImmutable`; luego `ApplyCatalogMutation(OpUpdate)` + `Validate`. Repository traduce duplicate/reference/not-found | `repo.Get` + posible `repo.ReferencedByResources` + `repo.Update`; `catalogAdminRepository.Update` es transaccional; `publish` sólo tras éxito |
| `Deactivate(ctx context.Context, kind domain.CatalogKindCode, id int64) error` | Kind + ID; sin payload de salida | Delega `setActive(..., false)`: `id == 0` -> `ErrInvalidArgument`; carga current, aplica `OpDeactivate`, valida; un kind sin setter de actividad en el snapshot devuelve `ErrSoftDeleteUnsupported` sin persistir | `repo.Get` -> `repo.SetActive`; `catalogAdminRepository.SetActive` hace BEGIN, handler `setActive`, UPDATE y COMMIT; después `publish` |
| `Reactivate(ctx context.Context, kind domain.CatalogKindCode, id int64) error` | Kind + ID; sin payload de salida | Igual que Deactivate con `OpReactivate`; `ResourceCatalog.Validate` impide publicar una estructura globalmente inválida; propaga not-found, unsupported y errores de referencia | `setActive(..., true)` -> mismo flujo PostgreSQL transaccional -> `publish` |
| `Delete(ctx context.Context, kind domain.CatalogKindCode, id int64) error` | Kind + ID; borrado físico de estructura | `id == 0` -> `ErrInvalidArgument`; carga current, `ApplyCatalogMutation(OpDelete)` y `Validate`. El servicio no repite internamente los probes del UI; una FK 23503 es el backstop `ErrCatalogInUse`; cero filas -> `ErrCatalogRecordNotFound` | `repo.Get` -> `repo.Delete`; `catalogAdminRepository.Delete` hace BEGIN, handler `del`, DELETE y COMMIT; `publish` sólo tras éxito |

**[HECHO]** Los métodos auxiliares que sostienen guards son `Dependencies(ctx, kind, id) ([]CatalogDependency, error)` -> `repo.Dependents` y `ReferencedByResources(ctx, kind, id) (bool, error)` -> método homónimo del puerto. Ninguno valida `id` en el servicio. La TUI los usa antes de confirmar delete o permitir cambios de código (`internal/tui/catalog_admin.go:catalogDependencyChecker`, `catalogReferenceChecker`).

```text
WRITE DE CATÁLOGO (Create/Update/Lifecycle/Delete)
CatalogAdminAdapter
 -> catalogo.Service (mutex)
 -> repo.Get cuando la operación necesita estado actual
 -> domain.ApplyCatalogMutation(snapshot copy, registry, mutation)
 -> ResourceCatalog.Validate
 -> domain.CatalogAdminRepository
 -> postgres.catalogAdminRepository
 -> handler fijo de kindTables -> BEGIN/SQL/COMMIT
 -> Service.publish(next)
 -> CatalogAuthority.Publish(next) + versión in-process
```

Referencias: `internal/app/catalogo/service.go:List`, `Get`, `Create`, `Update`, `Deactivate`, `Reactivate`, `setActive`, `Delete`, `publish`; `internal/domain/catalog_record.go:CatalogAdminRepository`; `internal/postgres/catalog_admin_repository.go:List`, `Get`, `Insert`, `Update`, `SetActive`, `Delete`, `mapCatalogWriteError`, `mapCatalogDeleteError`, `requireRow`.

**[HECHO]** Hay una asimetría concreta: la DB agregó `active` a las 11 estructuras, pero el comentario y `ApplyCatalogMutation` indican que varias structs Go todavía no tenían `Active` al diseñarse. En el estado actual, `AttributeDefinition`, `ResourceUnitPolicy`, `PresentationField`, `ResourceOptionSet`, `AttributeOptionRelation`, `ResourceAttribute` y `AttributeRule` sí muestran campos `Active` en loader/uso; sin embargo `catalog_mutation.go` todavía pasa `nil` como setter para varios kinds y trata `KindOptionSet` como no-op. Lifecycle administrativo homogéneo de los 11 kinds es **no verificado** y debe investigarse antes de exponerlo por otro delivery.

**[HECHO]** La coherencia de snapshot sólo cubre cambios hechos por este `catalogo.Service`. No existe polling/listener ni recarga desde PostgreSQL después del boot. Cambios externos o desde otro proceso no actualizan `CatalogAuthority` hasta reiniciar: es un riesgo directo para múltiples runtimes Pi.

### 4.8 Seguridad de reutilización

- **[HECHO]** `garfex_app` tiene SELECT/INSERT/UPDATE/DELETE sobre Resource y valores; sobre catálogo recibió escritura en 000003. Supplier prohíbe DELETE y permite SELECT/INSERT/UPDATE.
- **[HECHO]** No hay autenticación, autorización por usuario, tenant, rate limit ni audit trail de negocio implementados en el runtime actual.
- **[HECHO]** Los servicios aceptan `context.Context`, útil para cancelación/deadlines, pero la TUI usa frecuentemente `context.Background()` y no establece timeout.
- **[RECOMENDACIÓN]** Reutilizar lectura dentro del mismo trust boundary es viable. Exponerlo por red, MCP o Pi multiusuario requiere una capa exterior de identidad, policy, deadlines, límites y auditoría; no debe agregarse al dominio.

### 4.9 Tests y garantías de Resource Master

| Área | Evidencia existente |
|---|---|
| Dominio/canonicalización | identity, scope, codes, types, canonical writes, conductores, canalizaciones, option-set isolation |
| Catálogo | validate, query ordering/active filtering, authority defensive clone, mutation, presentation |
| Aplicación | service success/error, forged writes, lifecycle, pagination, session coherence |
| PostgreSQL unit | codecs, error mapping, search normalization, status predicates, loader classification/parity normalization |
| PostgreSQL integración | CRUD, identity migration, cardinalidad, lifecycle, set hydration, loader active flags, unit names, grants, catálogo admin |
| TUI | editor, lifecycle, pagination, workspace switching, navigation, palette, searchable select, multiselect, catalog wizard/admin/e2e, sanitize |
| CLI/config | run paths, fail-fast, redacción, DSN y variables |

**[HECHO]** Los tests de integración dependen de DSN de test y pueden omitirse si no está disponible; su presencia no prueba que hayan corrido en esta auditoría. Resultado actual de la suite: **no verificado**, porque por pedido no se ejecutaron tests.

---

## 5. Inventario TUI

### 5.1 Mapa funcional

| Componente | Función | Naturaleza |
|---|---|---|
| `Model`, `Update`, `View` | Estado Bubble Tea, routing de teclas, viewport, pantallas | UX pura con coordinación |
| `commands.go` | Árbol `/`, workspaces por clase activa y acciones | UX derivada de datos de dominio |
| `InteractionEngine` | Despacho síncrono, history y pending | Aplicación de interacción, no negocio |
| `ResourcesWorkspaceAdapter` | Search/detail/create/edit/duplicate/lifecycle | Mezcla delivery + orquestación de casos de uso |
| `CatalogAdminAdapter` | CRUD genérico, filtros, wizard y confirmaciones | Mezcla delivery + orquestación |
| `resource_editor_*` | Estado, transiciones, mapeo de intent y presentación | Principalmente UX; persistence mapping es frontera |
| `view.go` | Render y estilos | UX pura |
| `handlers.go` | Version/config/status y detalle técnico | Delivery |
| `sanitize.go` | Saneamiento de texto renderizado | Delivery/security de presentación |

### 5.2 Navegación y menús

**[HECHO]** `Model` maneja home, workspace, loading, result, error, min-size y manual. Home contiene Materiales Maestros, Versión, Verificar configuración, Estado y Salir (`model.go:New`). El constructor productivo `NewWithCatalogAuthority` agrega workspaces dinámicos: uno global `/recursos`, uno por clase activa y `/configuracion`.

**[HECHO]** En chat, `/` abre palette; `Esc` sale del workspace registrado o desenfoca el composer; `Enter` envía; Up/Down navega historial de prompts. `Ctrl+C` termina. En home, flechas y Enter navegan. Hay scroll PgUp/PgDown y manejo especializado de opciones/confirmaciones (`Model.Update`, `handlePaletteKey`, `handlePendingKey`). El soporte de `j/k` está cubierto por helpers/tests, pero su matriz completa por cada modo es **no verificado** sin ejecutar la TUI.

### 5.3 SearchableSelect, MultiSelect y GuidedInteraction

**[HECHO]** `QuestionRequest.SelectionMode` soporta single, multiple, free-text y searchable. `Option` incluye ID, label, value, description, target, metadata y search terms. `ConfirmationRequest`, `ActionRequest` y `StructuredResult` completan el contrato (`interaction.go`).

**[HECHO]** No existe un tipo llamado `GuidedInteraction`; la interacción guiada es el conjunto `InteractionAgent` + `InteractionEngine` + requests tipados + máquinas de estado en adapters. MultiSelect conserva orden de opciones y límites mínimos/máximos según `multiselect_test.go`; SearchableSelect está implementado en el modo searchable y cubierto por tests de preguntas/palette.

### 5.4 Estado, cachés, snapshots y refresh

- **[HECHO]** Cada workspace tiene `workspaceSlot.saved`; `snapshotWorkspace`/`applyWorkspace` preservan conversación, input, pending, selección y viewport al cambiar de workspace.
- **[HECHO]** `ResourcesWorkspaceAdapter` guarda `lastQuery`, `lastPage`, `lastDetail`, scope lifecycle y draft del editor. No mantiene una segunda caché de resultados; vuelve a ejecutar la búsqueda con criterios guardados.
- **[HECHO]** `InteractionEngine.history` y `Model.history` viven sólo en memoria del proceso; no hay persistencia de sesión.
- **[HECHO]** `refreshCatalogIfChanged` compara versión de `CatalogAuthority`, reconstruye workspaces/agentes de recursos, conserva Configuración, reinicia flujos abiertos de recursos y avisa al usuario (`model.go:636-667`).
- **[HECHO]** No existe refresh desde DB independiente de una mutación publicada en proceso. Reload multi-proceso: **no verificado**.

### 5.5 Presentación canónica

**[HECHO]** El título de recurso usa `recursos.Service.Describe` -> `ResourceCatalog.Describe`. Labels de atributos/opciones y unidad se enriquecen en TUI con el snapshot. La identidad técnica nunca debería mostrarse como nombre humano; tests como `TestKnownResourceCodesAreNeverRenderedOrTranslated` fijan esa separación.

---

## 6. Inventario IA/chat

### 6.1 Cinco preguntas de auditoría aplicadas

Para cada componente relevante se contestan literalmente: **(1)** problema que resuelve; **(2)** si depende del dominio GARFEX; **(3)** qué regla no debe perderse; **(4)** si Pi podría asumirla; **(5)** qué conservar si se retira el harness. “Pi podría” expresa sólo compatibilidad conceptual; la capacidad técnica concreta de Pi sigue **no verificado**.

| Componente | 1. Problema que resuelve | 2. ¿Depende del dominio GARFEX? | 3. Regla que no debe perderse | 4. ¿Pi podría asumirla? | 5. Qué conservar si se retira el harness |
|---|---|---|---|---|---|
| `AssistantShellAgent` (`assistant_shell_agent.go`) | Evita simular comprensión libre cuando no existe LLM y dirige de forma explícita a `/` | **No**; es dependency-free y no toca servicios/PostgreSQL | Nunca afirmar que interpretó lenguaje; las capacidades se eligen explícitamente | Podría asumir la shell **sólo** si mantiene esa honestidad y routing explícito; soporte concreto: **no verificado** | El texto/criterio de “sin falsa comprensión” y tests del greeting/respond |
| Contratos `InteractionInput`, `InteractionMessage`, `InteractionResponse` | Representan texto, selección, acción, cancelación, preguntas, confirmaciones y resultados sin acoplarlos a Bubble Tea messages ad hoc | **No directamente**; son contratos delivery genéricos, aunque payloads construidos por adapters reflejan operaciones GARFEX | Inputs y resultados deben permanecer tipados; `Pending` representa la interacción aún no resuelta; targets locales no son operaciones de agente | Pi podría mapearlos a su UI/protocolo; equivalencia concreta: **no verificado** | Semántica de tipos, selección single/multiple/searchable/free-text, confirmación, action target y pruebas de contrato |
| `InteractionAgent` + `InteractionEngine` | Despachan una entrada a una capacidad cerrada, convierten errores a `ErrorMessage` y mantienen history/pending efímeros | La interfaz/engine **no**; cada implementación puede depender de servicios GARFEX | `ActionTargetLocal` no se envía al agent; agent nil falla explícitamente; un error no debe filtrarse como éxito; history/pending son efímeros, no business truth | Pi podría asumir despacho e historial visual; su capacidad para respetar estas garantías es **no verificado** | La interfaz mínima `Respond(context.Context, InteractionInput)`, reglas de error/local target y tests `interaction_test.go` |
| Palette y routing (`commands.go`, `Model.openPalette`, `enterWorkspace`) | Descubren capacidades disponibles y enrutan por IDs/slugs deterministas, sin clasificador semántico | **Sí parcialmente**: workspaces/acciones se derivan de clases activas y `CatalogKind`; no ejecutan reglas de negocio | Sólo anunciar capacidades realmente cableadas; clases inactivas no crean workspaces; routing explícito no concede autoridad | Pi podría asumir descubrimiento/routing visual; discovery dinámico y equivalencia son **no verificado** | `BuildWorkspaceDescriptors`, árbol de acciones, IDs estables, filtros aliases/keywords y pruebas de routing |
| `ResourcesWorkspaceAdapter` | Coordina búsqueda, página, detalle, create/edit/duplicate y lifecycle de una capacidad Resource cerrada | **Sí**: usa `ResourceCatalog` y seis interfaces satisfechas por `recursos.Service` | Nunca validar como autoridad final en UI; usar clase + identity real; `SearchPage` conserva criterios; writes pasan por `NewResource`; lifecycle es explícito y confirmado | Pi podría asumir la **orquestación/UX**, no `NewResource`, `recursos.Service` ni repository; integración concreta: **no verificado** | Interfaces mínimas, comandos/DTOs de aplicación, estados y pruebas de flujos; conservar todo el Core domain/app/postgres |
| `CatalogAdminAdapter` + wizard | Ofrecen CRUD descriptor-driven para 11 kinds, selección de referencias, guards y confirmaciones | **Sí**: depende de `CatalogRegistry`, `CatalogRecord` y contratos de `catalogo.Service` | No saltar dependencias/referencias, inmutabilidad de código, `ApplyCatalogMutation`/`Validate`, confirmación y errores tipados | Pi podría asumir la UX; exponer mutaciones desde Pi no es recomendable todavía y su soporte es **no verificado** | Registry/descriptores, narrow interfaces, guards, wizard tests y `catalogo.Service` como única autoridad |
| `conversationMessage`, `workspaceState`, `snapshotWorkspace`/`applyWorkspace` | Preservan conversación, pending, selección, input y viewport al cambiar de workspace | **No para negocio**; guarda mensajes producidos por capacidades GARFEX, no entidades como autoridad | Estado conversacional es efímero; no reemplaza DB, no autoriza operaciones y no debe alterar el draft de otro workspace | Pi podría asumir memoria visual de sesión; aislamiento/restauración equivalentes: **no verificado** | Regla de aislamiento por workspace, estado mínimo que debe sobrevivir y tests de switch/viewport |
| Fakes y harness de tests (`fake_agent.go`, recording agents) | Prueban render, despacho, inputs, cancelación y navegación sin LLM ni DB | **No**, salvo fixtures específicos que simulan contratos GARFEX | Los tests deben seguir probando la frontera y no convertirse en evidencia falsa de provider/runtime real | Pi no necesita asumir los fakes; deberá aportar un seam testeable equivalente si reemplaza el harness | Casos y aserciones de contrato, especialmente que no haya llamadas ante cancelación/input inválido |
| `docs/architecture/agent-runtime-harness.md` y límite MCP | Definen principios futuros de control previo, capabilities, policy, evidencia y separación de business truth | **Sí conceptualmente**: preservan autoridad de servicios Go/PostgreSQL, pero no son código ejecutable | Modelo/prompt/Skill/MCP no autorizan ni agregan tools; Go valida/ejecuta; PostgreSQL conserva verdad; fallos cierran acceso | No puede afirmarse que Pi lo asuma: implementación concreta **no verificado** | El documento de decisiones, outcomes estables, límites de autoridad y criterios de evaluación; no presentar `CapabilityResolver`/`OperationPolicy`/`RunEnvelope` como implementados |

### 6.2 Clientes, providers y streaming

**[HECHO]** `go.mod` no contiene SDK de OpenAI, Anthropic, Gemini, ADK, MCP ni otro provider. No hay cliente HTTP de modelo, selección de provider, tokens, streaming, chunks, callbacks o persistencia de respuestas. Todo ello es **no verificado** como capacidad externa de Pi y ausente como implementación GARFEX.

### 6.3 Mensajes, conversaciones, sesiones e historial

**[HECHO]** Los mensajes tipados son `TextMessage`, `QuestionRequest`, `ConfirmationRequest`, `ActionRequest`, `StructuredResult` y `ErrorMessage`. `InteractionResponse` trae `Messages` y un único `Pending`. No hay IDs durables de conversación/sesión, timestamps, usuario, replay ni almacenamiento.

**[HECHO]** History existe dos veces con objetivos distintos: `InteractionEngine.history []InteractionMessage` y `Model.history []conversationMessage` para render/resolución. Ambos son memoria efímera y no son business truth.

### 6.4 Prompts, tools, agentes, router, eventos y contexto

- **Prompts:** **[HECHO]** sólo copy/preguntas deterministas y `FieldDescriptor.Guidance`; no hay prompt templates para modelos ni reglas de negocio sólo en prompts.
- **Tools:** **[HECHO]** no hay tool registry/runtime de producto. Las acciones TUI tipadas no son tools LLM.
- **Agentes:** **[HECHO]** el nombre `InteractionAgent` representa un adapter determinista; `AssistantShellAgent` no interpreta lenguaje; recursos y catálogo son capacidades cerradas.
- **Router/intenciones:** **[HECHO]** no hay clasificación semántica. El routing es explícito por palette/action IDs y workspace slug en `commands.go`/`Model`.
- **Comandos/eventos:** **[HECHO]** existen inputs/actions Bubble Tea y `tea.Msg`; no existe event bus de dominio ni command bus.
- **Contexto/memoria:** **[HECHO]** contexto es `context.Context` de Go y estado local. Engram sólo aparece en documentación de límites; no es dependencia de runtime.
- **Harness:** **[HECHO]** `docs/architecture/agent-runtime-harness.md` es explícitamente documentation-only. `CapabilityResolver`, `OperationPolicy` y `RunEnvelope` no existen como símbolos Go actuales.

---

## 7. Dependencias y acoplamientos

### 7.1 Dependencias externas

| Dependencia | Uso | Acoplamiento |
|---|---|---|
| Bubble Tea/Bubbles/Lip Gloss | TUI | Confinado a `internal/tui` y entrypoint |
| pgx v5 | Pool, tx, SQL y errores | Confinado a postgres, supplier postgres y composition root |
| shopspring/decimal | Valores DECIMAL/QUANTITY de dominio | Dependencia de tipo dentro del Core |
| `x/text` | Canonicalización Unicode | Dominio |
| PostgreSQL | Business truth y catálogos | Adaptadores/migraciones; requerido al arrancar TUI productiva |

### 7.2 Bypasses buscados

| Bypass | Resultado |
|---|---|
| TUI -> Repository/PostgreSQL | **[HECHO]** no detectado; TUI llama interfaces de servicio |
| Agente -> Repository/PostgreSQL | **[HECHO]** no detectado; adapters reciben servicios mínimos |
| Dominio -> TUI | **[HECHO]** no detectado |
| Servicio -> chat/Bubble Tea | **[HECHO]** no detectado |
| SQL fuera de infraestructura | **[HECHO]** productivo sólo en `internal/postgres`, supplier postgres, migraciones/scripts; SQL adicional está en tests de integración |
| Reglas sólo en prompts | **[HECHO]** no detectado |
| Repository público saltando servicio | **[HECHO]** `suppliers.Module.Repository` lo expone; sin caller productivo. Resource repository se construye en composition root y se entrega al servicio |
| Duplicación de reglas | **[HECHO]** DB y dominio duplican algunas constraints deliberadamente; TUI filtra valores aplicables pero app vuelve a validar. No se detectó autoridad exclusiva en TUI |

### 7.3 Acoplamientos relevantes

**[HECHO]** El dominio Resource y catálogo administrativo comparten el paquete global `internal/domain`, no un módulo `internal/modules/resources`. Esto facilita consistencia actual, pero hace menos explícito el bounded context que Supplier.

**[HECHO]** `CatalogRecord` es una unión tipada genérica basada en `map[string]CatalogValue`. Es neutral respecto de TUI/Postgres, pero los nombres de fields deben coincidir con registry, mapper de dominio y SQL. Agregar un kind exige cambios coordinados.

**[HECHO]** La TUI recibe un snapshot `ResourceCatalog` además de servicios; usa datos de dominio para construir preguntas y presentar. No viola persistencia, pero acopla el delivery al shape completo del catálogo.

---

## 8. Qué ya es reutilizable

### Qué reutilizaría hoy para GARFEX sobre Pi

| Forma | Componente | Condición |
|---|---|---|
| **Tal cual** | Reglas y tipos neutrales de `internal/domain` | Dentro del mismo módulo Go o tras publicar un package estable |
| **Tal cual** | `recursos.Service`, `catalogo.Service` | Inyección de puertos; no dependen de TUI |
| **Tal cual** | Repositorios PostgreSQL y `LoadResourceCatalog` | Backend Go con pgx y mismo esquema |
| **Tal cual** | Migraciones 000001-000007, roles y grants | PostgreSQL sigue siendo autoridad |
| **Compartir como dependencia** | `CatalogAuthority`, `CatalogRegistry`, `ResourceRepository` | Un solo proceso o lifecycle coherente |
| **Mediante adaptador** | Proyecciones de Resource/Catalog para Pi | Evitar filtrar structs internos y `CatalogRecord` genérico al cliente |
| **Reemplazar por Pi** | shell del assistant, render, palette, viewport, composer, snapshots visuales | Sólo si Pi ofrece UX equivalente; capacidad exacta no verificado |
| **No tocar todavía** | catálogo admin write/lifecycle, Supplier delivery, agent/MCP futuro | Falta policy/audit y hay dudas de lifecycle/multiproceso |

**[HECHO]** Los contratos más neutrales son `Resource`, `CreateCommand`, `UpdateCommand`, `SearchCriteria`, `ResourcePage`, `ResourceRepository`, entidades de catálogo y servicios app. Los acoplados a infraestructura son `*pgxpool.Pool`, SQL/scanners y `Config.DSN`; los acoplados a TUI son `InteractionMessage`, Bubble Tea `Model`, adapters y presentation helpers.

---

## 9. Qué debería consumir API

**[RECOMENDACIÓN]** Una API determinista debería exponer proyecciones versionadas sobre casos de uso, no repositories ni `CatalogRecord` crudo:

- clases/familias/tipos/unidades/características activas para navegación;
- búsqueda paginada de recursos con filtros y lifecycle explícito;
- detalle por identificador opaco o contrato estable de clase + identity;
- presentación canónica producida por `Service.Describe`;
- mutaciones sólo cuando haya identidad, autorización, idempotencia, auditoría y concurrencia definidas.

```text
Pi/Web/CLI -> API adapter -> recursos.Service / catalogo.Service
                         -> domain ports -> PostgreSQL adapters
```

**[HECHO]** No existe API HTTP/gRPC actual, DTO público, auth ni versionado de transport. Implementación y protocolo son **no verificado** y quedan fuera de esta auditoría.

---

## 10. Qué debería consumir MCP

**[RECOMENDACIÓN]** MCP sólo debería ofrecer capacidades estrechas para hosts de IA, inicialmente read-only: búsqueda y detalle. Debe llamar el mismo servicio que API/TUI, aplicar bounds/deadlines/policy y sanitizar respuestas.

**[HECHO]** `docs/architecture/mcp-capability-boundary.md` ya define conceptualmente `materials.search.v1` y `materials.get.v1`, dos tools y cero resources, sin SDK ni endpoint. Esa frontera coincide con el Core actual, pero sigue siendo documentación, no ejecución.

**[RECOMENDACIÓN]** MCP no debe exponer:

- `ResourceRepository`, `CatalogAdminRepository`, SQL o schema;
- CRUD genérico de 11 kinds;
- `Config.DSN` o secretos;
- mutaciones antes de policy/audit/confirmación;
- reglas reimplementadas en descripciones de tools o prompts.

---

## 11. Qué Pi podría reemplazar

**[RECOMENDACIÓN]** Pi podría reemplazar la capa exterior de experiencia:

- `AssistantShellAgent` placeholder;
- composer/chat visual, palette `/`, menús y routing de workspaces;
- render de `InteractionMessage`, searchable select, multiselect y confirmaciones;
- historial visual efímero y restauración de workspace;
- coordinación de capacidades cerradas si conserva contratos tipados y llamadas a servicios.

**[HECHO]** No hay evidencia en GARFEX de que Pi implemente Bubble Tea, persistence, tool policy o UI terminal equivalente. El reemplazo exacto es **no verificado** hasta auditar Pi y probar sus límites.

---

## 12. Qué NO debe moverse a Pi

**[RECOMENDACIÓN]** Deben permanecer en el Core Go/PostgreSQL:

1. `NewResource`, canonicalización, reglas condicionales, relaciones y validación activa.
2. Derivación/verificación de `IdentityKey` v1.
3. `CatalogAuthority`, `ResourceCatalog.Validate` y presentación canónica.
4. Semántica de lifecycle y filtros activos/inactivos.
5. Transacciones, cardinalidad, hidratación efectiva y error mapping.
6. Constraints, FKs, triggers, grants, roles y migraciones.
7. Casos de uso `recursos.Service` y `catalogo.Service`.
8. Supplier domain/app/postgres.
9. Redacción de secretos y construcción de DSN.

Mover estas reglas a prompts, tools Pi o estado de conversación crearía dos autoridades y permitiría bypasses.

---

## 13. Estrategia recomendada para reutilizar el Core

### Comparación explícita

| Alternativa | Ventajas según el código | Riesgos según el código | Juicio |
|---|---|---|---|
| **A. Copiar paquetes** | Inicio rápido; permite elegir sólo domain/app | Fork inmediato de identidad, reglas, errores y migraciones; `/internal/` incentiva la copia; paridad difícil | **No recomendada** |
| **B. Convertir Core en librería/módulo** | Reutilización in-process, tipos y servicios directos, bajo overhead | Requiere sacar contratos de `/internal/`, definir API estable y evitar exportar pgx/TUI | **Preferida si Pi corre Go en el mismo proceso** |
| **C. Comunicar Pi por API/RPC/MCP** | Mantiene GARFEX como autoridad, soporta otro lenguaje/proceso, deployment independiente | Auth, DTO, versionado, latencia y observabilidad aún no existen; MCP no sirve como API universal | **Preferida si Pi es proceso/runtime separado**; API/RPC para clientes deterministas, MCP sólo IA |
| **D. Mantener backend ejecutable y agregar adaptador local estrecho** | Menor cambio del Core; puede usar stdio/IPC tipado y conservar composition root | Lifecycle, cancelación y protocolo propios; puede volverse API informal | **Puente posible**, no destino sin contrato |

**[RECOMENDACIÓN]** La elección entre B y C depende de un dato externo: cómo se integra Pi. Eso es **no verificado**. En ambos casos, primero debe estabilizarse un boundary público de lectura con DTOs/proyecciones, sin mover reglas.

**[RECOMENDACIÓN]** No se propone aquí un plan de implementación ni PRs. La decisión de arquitectura es mantener una sola autoridad y seleccionar el mecanismo de consumo después de verificar Pi.

---

## 14. Primer vertical Recursos Maestros + Pi

### Alcance mínimo READ-ONLY

```text
Pi menú "Recursos"
 -> adaptador Pi/GARFEX
    ├─ listar clases
    │   -> catalogo.Service.List(ctx, KindClass, CatalogFilter{Status: Active})
    │   -> CatalogAdminRepository.List
    │   -> postgres.catalogAdminRepository
    │   -> PostgreSQL resource_classes
    ├─ listar familias/tipos/características/unidades
    │   -> catalogo.Service.List(ctx, kind, filtro parent/status)
    │   -> mismo repository -> tablas catalogográficas reales
    ├─ listar/buscar recursos
    │   -> recursos.Service.SearchPage(ctx, SearchCriteria{...})
    │   -> ResourcePageRepository.SearchPage
    │   -> PostgreSQL recursos + effective attributes
    └─ detalle
        -> recursos.Service.Get(ctx, classCode, identityKey)
        -> recursos.Service.Describe(resource)
        -> PostgreSQL + presentación canónica del CatalogAuthority
```

**[HECHO]** Todos los casos de uso y repositorios de este flujo existen hoy. La única pieza inexistente es el adaptador Pi y sus DTOs.

**[RECOMENDACIÓN]** El vertical debe excluir create/update/deactivate/reactivate, catálogo admin, Supplier, LLM, memoria y tools genéricas. No debe usar `SeedResourceCatalog`, acceder a tablas ni reconstruir nombres/identidad en Pi.

**[RECOMENDACIÓN]** Para detalle, la pareja `(classCode, identityKey)` es el contrato real actual. Si el límite externo no debe exponer una identity larga, el adaptador puede tratarla como opaca; inventar otro ID estable requiere una decisión separada.

---

## 15. Tabla final de componentes: conservar / adaptar / reemplazar / investigar

| Componente actual | Responsabilidad | Clasificación | Conservar | Adaptar | Pi podría reemplazarlo | Motivo |
|---|---|---:|---|---|---|---|
| `domain.NewResource` y tipos Resource | Reglas, identidad y agregado | A | Sí | No inicialmente | No | Autoridad de negocio probada |
| `ResourceCatalog.Validate/query/Describe` | Estructura, opciones y presentación | A | Sí | Boundary de lectura | No | Data-driven y testeado |
| `CatalogAuthority` | Snapshot/version coherente | B | Sí | Soporte multiproceso a investigar | No | Hoy sólo coherencia in-process |
| `recursos.Service` | Casos de uso Resource | A/C | Sí | Proyección pública | No | Independiente de TUI |
| `catalogo.Service` | Administración y lectura catálogo | B/C | Sí | Separar surface read/write externa | No | Genericidad útil; lifecycle parcial dudoso |
| `ResourceRepository` | Puerto neutral | A | Sí | Publicarlo fuera de `/internal/` si B | No | Inversión correcta |
| `postgres.resourceRepository` | Persistencia Resource | C | Sí | Factory/composición pública | No | SQL/transacciones reales |
| `postgres.catalogAdminRepository` | Persistencia catálogo | B/C | Sí | Limitar exposición exterior | No | Útil, pero CRUD amplio |
| `LoadResourceCatalog` | Snapshot consistente | C | Sí | Estrategia reload | No | Canonical runtime source |
| Migraciones/roles/grants | Integridad y permisos | A | Sí | Evolución normal | No | PostgreSQL es business truth |
| `ResourcesWorkspaceAdapter` | UX y coordinación Resource | B/D | Como referencia/tests | Extraer proyección | Sí | Mezcla delivery y estado |
| `CatalogAdminAdapter` | UX CRUD catálogo | B/D | Por ahora | No exponer en vertical inicial | Sí parcialmente | Delivery amplio y stateful |
| `InteractionMessage/Engine` | Protocolo/UI guiada | B/D | Si Pi lo puede renderizar | Bridge o DTO | Sí | No contiene negocio |
| `AssistantShellAgent` | Placeholder | D | Sólo hasta tener reemplazo | No | Sí | Declara que no hay IA |
| `Model/View/commands` | Bubble Tea UX | D | Mientras TUI siga soportada | Ninguna para Core | Sí | Capa exterior |
| `config` y `Secret` | Runtime seguro | A | Sí | Inyección por host si aplica | No | Independiente de UX |
| Supplier domain/app | Core Supplier | A | Sí | Boundary futuro | No | Bounded context independiente |
| Supplier postgres/module | Persistencia/composición | B/C | Sí | No exponer repository | No | No compuesto actualmente |
| Docs MCP/runtime | Límite futuro | B | Sí como decisión | Validar contra implementación futura | No aplica | No son runtime |
| `README.md` estado Fase 0 | Descripción del producto | E | No como hecho actual | Corregir en trabajo separado | No aplica | Contradicho por código y migraciones |
| OpenSpec checkboxes PR6-9 | Tracking histórico | E | Como historial | No usar como estado actual | No aplica | Contradicho por símbolos actuales |
| Otros candidatos E | Retiro | — | — | — | — | **[HECHO]** ninguno probado; no se recomienda retirar por mera alternativa |

---

## 16. Riesgos y dudas todavía no verificadas

1. **[NO VERIFICADO]** Capacidad real de integración de Pi: lenguaje, proceso, plugins, stdio/RPC, UI, auth y lifecycle no están en este repositorio.
2. **[NO VERIFICADO]** Resultado actual de tests/unit/integration y estado real de una DB migrada; no se ejecutaron por alcance READ-ONLY sin tests.
3. **[NO VERIFICADO]** Paridad byte-a-byte/semántica total entre `SeedResourceCatalog` y una DB real con 000001-000007 más cambios administrativos.
4. **[HECHO]** El catálogo no observa writes de otros procesos. Antes de múltiples runtimes se necesita decidir invalidación/reload; la conducta multi-proceso actual es **no verificado**.
5. **[HECHO]** Lifecycle administrativo de varios kinds presenta señales contradictorias entre structs cargados con `Active`, comentarios de `CatalogKind.SoftDelete` y setters `nil`/OptionSet no-op en `ApplyCatalogMutation`; comportamiento end-to-end de cada uno es **no verificado**.
6. **[HECHO]** No hay authn/authz, tenant, rate limiting ni auditoría de negocio. Seguridad para API/MCP/Pi multiusuario es **no verificado**.
7. **[HECHO]** `context.Background()` sin timeout aparece en handlers/adapters; comportamiento ante DB lenta/cancelación desde Pi es **no verificado**.
8. **[HECHO]** `Supplier Master` está implementado pero no compuesto; su integración futura con productos/precios/Resource es **no verificado** y no debe inferirse.
9. **[HECHO]** Todo el Core está bajo `/internal/`; viabilidad de librería B requiere un boundary público aún inexistente.
10. **[HECHO]** `README.md` y el OpenSpec de deuda están desactualizados; cualquier decisión basada sólo en ellos puede clasificar erróneamente capacidades ya presentes.
11. **[NO VERIFICADO]** Compatibilidad de contratos actuales con concurrencia optimista, múltiples writers o versionado de API; no existen ETags/version fields de agregado.
12. **[NO VERIFICADO]** Permisos efectivos de una instalación ya existente que haya recorrido upgrades parciales; los grants esperados sí están en migraciones, pero no se consultó una DB.

### Dictamen final

**[HECHO]** GARFEX tiene un Core Resource Master reutilizable y separado de su TUI en los puntos esenciales. **[RECOMENDACIÓN]** Pi debe comenzar como consumidor read-only por un adaptador estrecho; reemplazar UX es razonable, reemplazar reglas o persistencia no lo es. La decisión librería versus API/RPC depende de verificar el modelo de integración de Pi, sin copiar paquetes ni crear una segunda autoridad.
