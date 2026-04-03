# hurl-sync

Keep [hurl](https://hurl.dev) test files in sync with your Swagger/OpenAPI spec.

`hurl-sync` reads a `swagger.json` (Swagger 2.0 or OpenAPI 3.0), compares it against your existing `.hurl` files, and reports drift or auto-fixes it.

## Install

```bash
go install github.com/chamhaw/hurl-sync@latest
```

## Usage

```
hurl-sync check  --swagger <path> --dir <path>
hurl-sync sync   --swagger <path> --dir <path> [--dry-run]
```

### `check` — Report coverage and drift

Compares every endpoint in the spec against hurl files and prints a status for each:

| Status | Meaning |
|---|---|
| `OK` | Hurl file exists, spec-hash matches, file is in the correct directory |
| `MISSING` | No hurl file found for this endpoint |
| `STALE` | Hurl file exists but spec-hash does not match (spec changed) |
| `MISPLACED` | Hurl file exists and is up-to-date but lives in the wrong directory |
| `ORPHAN` | Hurl file references an operationId that no longer exists in the spec |

Exit code is **0** if all endpoints are `OK` or `STALE`, **1** otherwise.

### `sync` — Auto-fix drift

Applies fixes for each status:

| Status | Action |
|---|---|
| `MISSING` | Creates a skeleton `.hurl` file |
| `STALE` (body unchanged) | Overwrites the file with a fresh skeleton |
| `STALE` (body changed) | Merges new fields into the existing file, preserving user values |
| `MISPLACED` | Moves the file (and companion `.json`) to the correct directory |
| `ORPHAN` | Deletes the file |

Use `--dry-run` to preview changes without writing to disk.

## Hurl file header format

Each managed hurl file starts with metadata comments:

```hurl
# operationId: getSandboxPool
# tag: SandboxPool
# spec-hash: a1b2c3d4e5f6a7b8
# request-hash: 1234abcd5678efgh
GET {{base_url}}/api/v1/sandboxpools/{{name}}
...
```

- **operationId** — links the file to a spec endpoint
- **tag** — determines the subdirectory
- **spec-hash** — hash of path + method + parameters + response schema (detects spec changes)
- **request-hash** — hash of the request body schema only (determines overwrite vs merge on stale)

## Directory layout

Files are organized by swagger tag in kebab-case:

```
test/hurl/
  sandbox-pool/
    get-sandbox-pool.hurl
    create-sandbox-pool.hurl
  workspace/
    create-workspace.hurl
    get-workspace.hurl
```

File names are the operationId converted to kebab-case with a `.hurl` extension.

## License

MIT
