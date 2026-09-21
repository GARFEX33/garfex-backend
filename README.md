# GARFEX Costos Unitarios

Core hexagonal (dominio + casos de uso + adaptador PostgreSQL) del Resource
Master, Supplier Master y Purchase and Price History Core, sin ninguna capa de
interfaz (TUI, CLI, Web, API, MCP o Agentes) — esas capas viven fuera de este
repositorio.

## Arquitectura

- [Límite de capacidades MCP](docs/architecture/mcp-capability-boundary.md)
- [Inicialización pública del Resource Master Core](docs/architecture/resource-master-core.md)
- [Harness de runtime de agentes](docs/architecture/agent-runtime-harness.md)

## Consumo como librería Core

El punto de entrada público es el paquete raíz `garfex`. Abrí una instancia con
un DSN explícito y cerrala cuando termine su uso:

```go
import (
	"context"
	"fmt"

	garfex "github.com/GARFEX33/garfex-costos-unitarios"
)

func run(ctx context.Context, dsn string) error {
	app, err := garfex.Open(ctx, garfex.Config{DSN: dsn})
	if err != nil {
		return fmt.Errorf("open GARFEX Core: %w", err)
	}
	defer app.Close()

	return nil
}
```

`Config.DSN` es obligatorio. Core no carga implícitamente variables `GARFEX_*`
ni archivos `.env`; el host debe proporcionar un DSN completo. Al analizarlo,
pgx puede aplicar sus valores predeterminados documentados `PG*` únicamente a
los parámetros de conexión omitidos. Usá la identidad runtime `garfex_app`;
las migraciones son una tarea externa de administración con `garfex_admin`.

`Application` expone exactamente estos seis handles públicos:

- `ResourceReader` (`*resourcecore.Reader`)
- `ResourceWriter` (`*resourcecore.Writer`)
- `SupplierReader` (`*suppliercore.Reader`)
- `SupplierWriter` (`*suppliercore.Writer`)
- `PurchaseReader` (`*purchasecore.Reader`)
- `PurchaseWriter` (`*purchasecore.Writer`)

Purchase and Price History Core reutiliza `SupplierProduct.CurrentMapping` como
conocimiento confirmado. Sus escrituras son semánticas, protegidas por revisión
y auditadas; el estado efectivo de cada línea se deriva de su override, del
mapeo actual y de `Resource.Active`. Consultá `purchasecore/doc.go` para el
contrato completo.

Para Resource Master, designá un único proceso writer autoritativo. Cada reader
observa un snapshot coherente de su última lectura; otro proceso necesita una
reconstrucción o reinicio explícito para observar escrituras externas. No hay
recarga automática y `Resource ScopeAll` todavía no está soportado. Consultá el
contrato y las restricciones completas en
[Resource Master Core](docs/architecture/resource-master-core.md).

## Lectura de CFDI

`cfdicore` es un paquete puro (sin base de datos, fuera de `Application`) que
lee comprobantes CFDI 4.0 del SAT: `cfdicore.Parse(xmlBytes)` devuelve un
`cfdicore.Invoice` con encabezado, emisor, receptor, conceptos con impuestos,
impuestos globales y timbre fiscal. Solo parsea; no valida sellos ni persiste
nada. Los errores son tipos estables (`INVALID_XML`, `NOT_CFDI`,
`UNSUPPORTED_VERSION`, `INVALID_CFDI`). El contrato completo está en
`cfdicore/doc.go`.

## Preparación de versión

Primera versión publicada: `v0.1.0`. Un consumidor externo la referencia
directamente, sin `replace` local:

```go
require github.com/GARFEX33/garfex-costos-unitarios v0.1.0
```

Para desarrollo local contra un checkout sin publicar, seguí usando un
`replace` local en `go.mod`:

```go
replace github.com/GARFEX33/garfex-costos-unitarios => ../garfex-costos-unitarios-workspace
```

Ese reemplazo es local y no portable como mecanismo de release. Los consumidores
externos reproducibles deben fijar una versión publicada, no una referencia
móvil a `main`.

## Repositorios y remotos Git

El código del producto usa el repositorio público como fuente canónica. El
repositorio privado del workspace sólo almacena artefactos locales que no
pertenecen al producto.

| Remoto | Responsabilidad |
| --- | --- |
| `origin` | Repositorio público canónico. Aloja `main`, ramas de producto, pull requests, CI y releases. |
| `workspace` | Repositorio privado opcional para artefactos del entorno de trabajo. No es la base de ramas de producto. |

La rama local `main` debe seguir `origin/main`. El flujo normal comienza desde
esa referencia:

```powershell
git switch main
git pull --ff-only origin main
git switch -c <tipo>/<descripcion>
```

Verificá la configuración después de clonar o modificar remotos:

```powershell
git remote -v
git branch -vv
```

Los archivos privados del workspace no deben incorporarse a ramas destinadas
al repositorio público.

## Requisitos

- Go 1.26.5 (declarado en `go.mod`).
- Docker Engine con Docker Compose v2 para PostgreSQL y la herramienta de migración.

## Configuración

Copiá `.env.example` a `.env` y reemplazá cada valor vacío o `CHANGE_ME`; `.env` está ignorado por Git y Compose lo carga automáticamente.

| Grupo | Variables | Uso |
| --- | --- | --- |
| Runtime | `GARFEX_DB_HOST`, `GARFEX_DB_PORT`, `GARFEX_DB_NAME`, `GARFEX_DB_USER`, `GARFEX_DB_PASSWORD`, `GARFEX_DB_SSLMODE` | Conexión a PostgreSQL para el adaptador de persistencia y los tests de integración. |
| Runtime | `GARFEX_LOG_LEVEL` | Opcional: `debug`, `info`, `warn` o `error`; omitir equivale a `info`. |
| Bootstrap Compose | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Crea el cluster PostgreSQL. |
| Roles Compose | `GARFEX_ADMIN_PASSWORD`, `GARFEX_APP_PASSWORD` | Contraseñas de los roles de migración y runtime. |

## PostgreSQL local

Con `.env` completo, levantá y verificá la base:

```powershell
docker compose up -d --wait db
docker compose ps
docker compose restart db
```

El único servicio es `db`, publicado en `127.0.0.1:5432`, con el volumen nombrado `garfex_pgdata`; los datos persisten al reiniciar el servicio. El bootstrap crea `garfex_admin` (dueño de `public`, para migraciones) y `garfex_app` (runtime, solo `USAGE` en `public`); ambos son `NOSUPERUSER`, sin `CREATEDB` ni `CREATEROLE`. `POSTGRES_USER` es solo bootstrap y no debe usarse por la aplicación.

Para borrar intencionalmente la base local y volver a ejecutar los scripts de inicialización, usá `docker compose down -v`. Ese comando destruye `garfex_pgdata`.

## Migraciones

Las migraciones versionadas están en `migrations/`. La imagen fijada para ejecutarlas se puede verificar con:

```powershell
docker run --rm migrate/migrate:v4.18.2 -version
```

Ejecutalas con el DSN de `garfex_admin`; no uses `garfex_app` para administrar el esquema.

## Verificaciones

Los equivalentes locales de los controles no constructivos de CI son:

```powershell
gofmt -l .
go vet ./...
golangci-lint run ./...
go test ./... -count=1
docker compose config -q
```

`golangci-lint` requiere una versión compatible con Go 1.26.5 (probado con v2.12.2); instalala con `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`. Su configuración vive en `.golangci.yml`.

CI ejecuta además `go test ./... -race -count=1` y `go build ./...`.
