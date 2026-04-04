# API Server

Build a REST API server with CRUD operations for an in-memory data store.

Create `main.go` with:
- `GET /items` - list all items
- `POST /items` - create a new item (JSON body: `{"name": "..."}`)
- `GET /items/{id}` - get item by ID
- `DELETE /items/{id}` - delete item by ID

Each item has: `id` (auto-increment int), `name`, `created_at` (timestamp)

Use only standard library (net/http, encoding/json). No external dependencies.
