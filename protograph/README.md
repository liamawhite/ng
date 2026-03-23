# protograph

A GraphQL-like query layer over gRPC. One HTTP POST fans out to multiple gRPC calls, stitches the results together, and returns a single field-masked JSON response.

## The problem it solves

A typical client that wants "all areas with their projects and tasks" must issue requests serially:

```
GET /api/v1/areas
  GET /api/v1/projects?area_id=a1
    GET /api/v1/tasks?project_id=p1
    GET /api/v1/tasks?project_id=p2
  GET /api/v1/projects?area_id=a2
    ...
```

Protograph collapses this into a single request. The gateway handles all fan-out internally, running sibling calls in parallel.

## Concepts

### The annotation

Relations are declared on the foreign-key field of the child entity using `(protograph.v1alpha1.parent)`:

```protobuf
import "protograph/v1alpha1/options.proto";

message Task {
  string project_id = 4 [(protograph.v1alpha1.parent) = { type: "ng.v1.Project" }];
}
```

One annotation encodes **two** traversal directions:

| Direction | Derived call | Result |
|-----------|-------------|--------|
| Forward (parent → children) | `TaskService.List(project_id=X)` once per parent | stitches `tasks: [...]` onto each Project |
| Reverse (child → parent) | `ProjectService.Get(id=task.project_id)` once per unique FK | stitches `project: {...}` onto each Task |

No dedicated batch RPCs to implement. The gateway drives the existing `List` and `Get` methods.

### Convention-based defaults

The gateway derives everything from the type name when fields 3–6 are absent:

| Annotation field | Default |
|-----------------|---------|
| `list_service` | `{pkg}.{ChildTypeName}Service` |
| `list_method` | `"List"` |
| `get_service` | `{pkg}.{ParentTypeName}Service` |
| `get_method` | `"Get"` |

Override any of these when a service does not follow the convention:

```protobuf
message Comment {
  string article_id = 1 [(protograph.v1alpha1.parent) = {
    type:         "blog.v1.Article"
    list_service: "blog.v1.ContentService"
    list_method:  "ListComments"
    get_service:  "blog.v1.ContentService"
    get_method:   "FetchArticle"
  }];
}
```

### The `via` field

If the filter field name in the `List` request differs from the FK field name on the entity, use `via`:

```protobuf
// FK field is "owner_id" but the ListRequest filter is "user_id"
string owner_id = 3 [(protograph.v1alpha1.parent) = { type: "...", via: "user_id" }];
```

## Query wire format

```
POST /protograph/v1alpha1/query
Content-Type: application/json

{
  "<fully.qualified.ServiceName>": {
    "<methodName>": {
      "$": { <request params> },
      "<responseField>": {
        "<subField>": {},
        ...
      }
    }
  }
}
```

- `$` — the RPC request parameters (empty `{}` for no params)
- Every other key — a response field to include
- Nesting a key requests a relation traversal into that field
- `{}` on a leaf means "include this scalar"
- Method names accept both `PascalCase` (`"List"`) and `lowerCamelCase` (`"list"`)
- The response mirrors the query structure exactly, keyed by the same service/method names you sent

## Examples

### Top-down: areas → projects → tasks

```json
{
  "ng.v1.AreaService": {
    "list": {
      "$": {},
      "areas": {
        "id": {},
        "title": {},
        "projects": {
          "id": {},
          "title": {},
          "tasks": {
            "id": {},
            "title": {},
            "status": {}
          }
        }
      }
    }
  }
}
```

Execution (all sibling calls run in parallel):
1. `AreaService.List()` — fetch all areas
2. `ProjectService.List(area_id=X)` — once per area
3. `TaskService.List(project_id=Y)` — once per project

Response:
```json
{
  "ng.v1.AreaService": {
    "list": {
      "areas": [
        {
          "id": "area-1",
          "title": "Work",
          "projects": [
            {
              "id": "proj-1",
              "title": "Alpha",
              "tasks": [
                { "id": "task-1", "title": "Write tests", "status": 1 }
              ]
            }
          ]
        }
      ]
    }
  }
}
```

### Bottom-up: list tasks and fetch their parent project

```json
{
  "ng.v1.TaskService": {
    "list": {
      "$": {},
      "tasks": {
        "id": {},
        "title": {},
        "status": {},
        "project": {
          "id": {},
          "title": {}
        }
      }
    }
  }
}
```

`ProjectService.Get` is called once per **unique** `project_id` value across all tasks — 10 tasks that all belong to the same project result in a single `Get` call.

### With params: filtered query + mixed traversal directions

```json
{
  "ng.v1.ProjectService": {
    "list": {
      "$": { "area_id": "area-abc", "status": 1 },
      "projects": {
        "id": {},
        "title": {},
        "tasks": { "id": {}, "title": {} },
        "area": { "id": {}, "title": {} }
      }
    }
  }
}
```

Forward (`tasks`) and reverse (`area`) fan-outs fire in the same pass.

### Self-referential: sub-projects

```json
{
  "ng.v1.ProjectService": {
    "list": {
      "$": {},
      "projects": {
        "id": {},
        "title": {},
        "projects": {
          "id": {},
          "title": {}
        }
      }
    }
  }
}
```

The inner `projects` uses the forward resolver for `Project.parent_id`, calling `ProjectService.List(parent_id=X)` once per top-level project.

## Wiring it up

```go
import (
    api "github.com/liamawhite/ng/api/golang"
    _ "github.com/liamawhite/ng/api/golang/protograph/v1alpha1" // registers extension
    "github.com/liamawhite/ng/protograph/pkg/gateway"
)

pg, err := gateway.New([]gateway.ServiceRegistration{{
    Conn: grpcConn,
    Files: []protoreflect.FileDescriptor{
        api.File_areas_proto,
        api.File_projects_proto,
        api.File_tasks_proto,
    },
}})

mux := http.NewServeMux()
mux.Handle("/api/", restGateway)
mux.Handle("/protograph/", pg)
```

The blank import is required to register the `(protograph.v1alpha1.parent)` extension in the global proto registry.

## TypeScript client

`protograph-gen-ts` is a `protoc` plugin that generates a typed client from your `.proto` files. Run it via `buf generate`.

```typescript
import { createClient } from './api/ts/ng_protograph'

const pg = createClient('http://localhost:8080')

const result = await pg.fetch({
  'ng.v1.AreaService': {
    list: {
      $: {},
      areas: {
        id: {},
        title: {},
        projects: { id: {}, title: {}, tasks: { id: {}, title: {}, status: {} } }
      }
    }
  }
})

// result['ng.v1.AreaService']?.list?.areas?.[0]?.projects?.[0]?.tasks
// fully typed as TaskResult[] | undefined
```

The generic runtime lives in `protograph/ts/client.ts` and `protograph/ts/types.ts` — no project-specific code. The generated file (e.g. `api/ts/ng_protograph.ts`) provides the typed `*Fields` interfaces for query construction and `*Result` interfaces for response consumption.

## Module layout

```
protograph/
  pkg/gateway/
    gateway.go      — ServiceRegistration, Gateway, ServeHTTP
    query.go        — ParseQuery, FieldSelection, MethodQuery
    executor.go     — fan-out, stitching, field masking
    descriptor.go   — ExtractRelations, RelationResolver, ReverseResolver
  cmd/protograph-gen-ts/
    main.go         — protoc plugin: reads FileDescriptorProtos, emits TypeScript
  ts/
    client.ts       — generic createClient() runtime
    types.ts        — FieldSelection, MethodQuery, Query types
```

## Limitations and TODOs

### Cycles in the query are impossible; cycles in data are safe

The query is a finite JSON tree. The executor recurses strictly into sub-selections, so it terminates when the selection runs out. Circular data (project A's `parent_id` points to B, B's back to A) is fine — the gateway only traverses as deep as the query asks.

### Fan-out explosion

A deep query over a large dataset issues many gRPC calls:

```
100 areas × 100 projects × 100 tasks = 10,101 gRPC calls
```

Sibling calls run in parallel, but the total number grows multiplicatively with data size and query depth. There is currently no limit on either. **TODO**: add configurable max fan-out count and max query depth, returning an error when exceeded.

### Single connection

`gateway.New` accepts multiple `ServiceRegistration` values but currently routes all calls through the first connection's `*grpc.ClientConn`. **TODO**: extend `ServiceRegistration` to carry per-service routing so different services can live on different backends.

### Reverse traversal does not propagate the parent's message type

When a reverse resolver stitches a parent object onto a child (e.g. `project.area`), the recursive `maskAndFanOut` call receives an empty message type string. This means the gateway cannot further fan-out relations *from* that stitched parent. For example, `tasks → project → subProjects` would not work. **TODO**: thread the `ParentMsgType` from `ReverseResolver` through the `map[string]any` branch in `maskAndFanOut`.

### No streaming

The gateway buffers the full response before sending. Large result sets are held entirely in memory. **TODO**: consider streaming or pagination support for large fan-outs.

### Pluralisation is naive

The forward relation field name is derived as `strings.ToLower(ChildTypeName) + "s"` (e.g. `Task` → `tasks`). This breaks for irregular plurals (`Person` → `persons`, not `people`). **TODO**: either accept it as a known limitation of the convention or expose a `field_name` override in `ParentRef`.
