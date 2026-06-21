# Golang Architecture Convention

## Purpose

This document defines the architecture, package structure, dependency rules, naming conventions, and coding standards for this application.

Goals:

- Consistent code organization
- Clean Architecture compliance
- Package by Feature structure
- Predictable file placement
- AI-agent friendly conventions
- High maintainability
- Idiomatic Go design

---

# Core Principles

This application follows:

- Clean Architecture
- Package by Feature
- SOLID principles where appropriate
- Explicit dependencies
- Composition over inheritance
- Small interfaces
- Idiomatic Go

---

# Golden Rules

---

## Rule 1 — Organize by Feature

Code MUST be grouped by business capability first.

✅ GOOD

```text
internal/modules/user
internal/modules/order
internal/modules/payment
```

❌ BAD

```text
internal/controllers
internal/services
internal/repositories
```

---

## Rule 2 — Dependencies Point Inward

Allowed dependency direction:

```text
delivery -> application -> domain
infrastructure -> domain
```

NEVER reverse dependencies.

❌ Forbidden

- domain importing infrastructure
- domain importing Gin/Echo/Fiber
- application importing HTTP handlers
- application importing database drivers directly

---

## Rule 3 — Domain Must Stay Pure

Domain layer MUST NOT depend on:

- HTTP
- databases
- ORM
- frameworks
- external APIs
- gRPC
- Redis
- Kafka

Domain contains only:

- entities
- business rules
- repository contracts
- domain services
- value objects

---

## Rule 4 — One Use Case = One Business Action

Each use case should represent one business action only.

Examples:

- CreateUser
- CancelOrder
- ApprovePayment

Use cases orchestrate workflows.

Use cases MUST NOT contain:

- HTTP handling
- SQL queries
- framework logic

---

# Project Structure

```text
internal/
│
├── modules/
│   │
│   ├── user/
│   │   │
│   │   ├── domain/
│   │   │   ├── entity/
│   │   │   ├── repository/
│   │   │   ├── service/
│   │   │   ├── valueobject/
│   │   │   └── errors/
│   │   │
│   │   ├── application/
│   │   │   ├── usecase/
│   │   │   ├── dto/
│   │   │   └── mapper/
│   │   │
│   │   ├── infrastructure/
│   │   │   ├── persistence/
│   │   │   ├── repository/
│   │   │   ├── external/
│   │   │   └── config/
│   │   │
│   │   ├── delivery/
│   │   │   ├── http/
│   │   │   ├── grpc/
│   │   │   └── websocket/
│   │   │
│   │   └── tests/
│   │
│   └── order/
│
├── shared/
│   ├── logger/
│   ├── database/
│   ├── errors/
│   ├── response/
│   └── utils/
│
├── bootstrap/
├── config/
└── server/
```

---

# Layer Responsibilities

---

# Domain Layer

Location:

```text
internal/modules/<feature>/domain
```

Purpose:

Contains pure business logic.

Contains:

- entities
- repository interfaces
- value objects
- domain services
- business rules

Rules:

- no framework imports
- no SQL
- no HTTP
- no JSON binding
- no ORM tags

✅ GOOD

```go
type User struct {
    id    string
    email Email
}

func (u *User) ChangeEmail(email Email) {
    u.email = email
}
```

❌ BAD

```go
type User struct {
    ID string `gorm:"primaryKey"`
}
```

---

# Application Layer

Location:

```text
internal/modules/<feature>/application
```

Purpose:

Coordinates business workflows.

Contains:

- use cases
- DTOs
- orchestration logic
- transaction coordination

Rules:

- application depends on domain
- application must not depend on delivery
- application should not know HTTP concepts
- use cases MUST return DTOs, never domain entities directly

Example:

```go
type CreateUserUseCase struct {
    userRepo domain.UserRepository
}

func (uc *CreateUserUseCase) Execute(
    ctx context.Context,
    input CreateUserInput,
) (*CreateUserOutput, error) {
}
```

---

# Infrastructure Layer

Location:

```text
internal/modules/<feature>/infrastructure
```

Purpose:

Implements external concerns.

Contains:

- database implementations
- Redis
- Kafka
- external APIs
- repository implementations

Rules:

- infrastructure implements domain contracts
- infrastructure may use frameworks/libraries

Example:

```go
type PostgresUserRepository struct {
    db *sql.DB
}
```

---

# Delivery Layer

Location:

```text
internal/modules/<feature>/delivery
```

Purpose:

Handles transport layer.

Contains:

- HTTP handlers
- gRPC handlers
- route registration
- request validation
- response formatting

Rules:

- handlers should stay thin
- handlers call use cases only
- no business logic in handlers

---

# Naming Conventions

---

# Package Naming

Use:

- lowercase
- short names
- singular when appropriate

✅ GOOD

```text
user
order
payment
repository
entity
```

❌ BAD

```text
UserModule
Repositories
UtilsPackage
```

---

# File Naming

Use snake_case.

Examples:

```text
create_user.go
user_repository.go
postgres_user_repository.go
http_handler.go
```

---

# Interface Naming

Prefer behavior-based names.

✅ GOOD

```go
type UserRepository interface {}
type PasswordHasher interface {}
```

❌ BAD

```go
type IUserRepository interface {}
```

---

# Struct Naming

| Type | Convention |
|---|---|
| Entity | User |
| Use Case | CreateUserUseCase |
| Repository | PostgresUserRepository |
| Handler | UserHandler |
| DTO | CreateUserInput |
| Mapper | UserMapper |

---

# Interface Rules

Prefer small interfaces.

✅ GOOD

```go
type UserRepository interface {
    FindByID(ctx context.Context, id string) (*User, error)
}
```

❌ BAD

```go
type UserRepository interface {
    Create()
    Update()
    Delete()
    Find()
    FindAll()
    Count()
}
```

---

# Dependency Injection Rules

Use constructor injection only.

✅ GOOD

```go
func NewCreateUserUseCase(
    repo domain.UserRepository,
) *CreateUserUseCase {
}
```

❌ BAD

```go
var globalRepo *Repository
```

---

# Context Rules

Pass context.Context explicitly.

✅ GOOD

```go
func (r *UserRepository) FindByID(
    ctx context.Context,
    id string,
) (*User, error)
```

❌ BAD

```go
func (r *UserRepository) FindByID(id string)
```

---

# Error Handling Rules

Use explicit errors.

Prefer:

```go
var ErrUserNotFound = errors.New("user not found")
```

Custom errors allowed for domain cases.

Do NOT use panic for business errors.

---

# DTO Rules

DTOs belong in application or delivery layer.

DTOs MUST NOT leak into domain.

Use cases MUST return DTOs — never return raw domain entities or structs.

✅ GOOD

```go
func (uc *GetUserUseCase) Execute(ctx context.Context, id string) (*GetUserOutput, error) {
    user, err := uc.userRepo.FindByID(ctx, id)
    // ...
    return mapper.ToDTO(user), nil
}
```

❌ BAD

```go
func (uc *GetUserUseCase) Execute(ctx context.Context, id string) (*User, error) {
    return uc.userRepo.FindByID(ctx, id)
}
```

Flow:

```text
HTTP Request
→ Request DTO
→ Use Case Input
→ Domain
→ Use Case Output (Response DTO)
→ HTTP Response
```

---

# Database Rules

Database models belong in infrastructure only.

Do NOT expose database entities outside infrastructure.

Infrastructure maps DB model ↔ domain entity.

---

# Transaction Rules

Transactions belong in application layer orchestration.

Repositories should not start transactions internally unless explicitly required.

---

# Validation Rules

Validation layers:

| Layer | Responsibility |
|---|---|
| Delivery | request validation |
| Application | workflow validation |
| Domain | business invariants |

---

# Testing Rules

---

# Domain Tests

Type:

- pure unit tests

Rules:

- no database
- no HTTP
- no external services

---

# Application Tests

Type:

- use case tests

Rules:

- use mocks/fakes
- test orchestration

---

# Infrastructure Tests

Type:

- integration tests

Rules:

- real database allowed
- external services allowed

---

# Delivery Tests

Type:

- API tests
- handler tests

---

# Logging Rules

Use structured logging.

✅ GOOD

```go
logger.Info("user created",
    "user_id", user.ID,
)
```

❌ BAD

```go
log.Println("created user")
```

---

# Shared Package Rules

Shared packages MUST remain generic.

Allowed:

```text
shared/logger
shared/database
shared/errors
```

Forbidden:

- business logic
- feature-specific code

---

# Import Rules

---

# Domain

Can import:

- standard library
- shared kernel

Cannot import:

- infrastructure
- delivery
- frameworks

---

# Application

Can import:

- domain
- shared

Cannot import:

- delivery

---

# Infrastructure

Can import:

- domain
- application
- third-party libraries

---

# Delivery

Can import:

- application
- shared

Should avoid direct infrastructure imports.

---

# AI Agent Instructions

When generating code:

1. Determine the feature first
2. Place files inside the correct module
3. Respect dependency direction
4. Keep handlers thin
5. Keep domain pure
6. Put orchestration in use cases
7. Put integrations in infrastructure
8. Avoid circular dependencies
9. Follow naming conventions strictly
10. Prefer explicit code over magic abstractions

---

# Example Feature

```text
internal/modules/user/
│
├── domain/
│   ├── entity/
│   │   └── user.go
│   │
│   ├── repository/
│   │   └── user_repository.go
│   │
│   └── errors/
│       └── errors.go
│
├── application/
│   ├── dto/
│   │   └── create_user_input.go
│   │
│   └── usecase/
│       └── create_user.go
│
├── infrastructure/
│   └── repository/
│       └── postgres_user_repository.go
│
├── delivery/
│   └── http/
│       ├── handler.go
│       └── routes.go
│
└── tests/
```

---

# Anti-Patterns

Avoid:

- fat handlers
- god services
- shared mega-utils
- framework leakage into domain
- direct SQL in handlers
- generic repository abstraction everywhere
- cyclic dependencies
- hidden magic

---

# Preferred Go Style

Prefer:

- explicit code
- small packages
- composition
- simple interfaces
- clear ownership
- readability over cleverness

---

# Final Decision Guide

If unsure where code belongs:

1. Is it business logic?
   → domain

2. Is it workflow orchestration?
   → application

3. Is it external integration?
   → infrastructure

4. Is it transport layer logic?
   → delivery
