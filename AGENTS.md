# Agent Guidelines for Event Registration Service

This is a Go-based event registration backend service using AWS SAM, DynamoDB, and Stripe for payments.

## Build Commands

```bash
# Build the project (generates code and builds SAM app)
make build

# Run locally (builds, starts SAM local API)
make local

# Generate Go code from OpenAPI spec
go generate ./...

# Run all tests
go test ./...

# Run tests for a specific package
go test ./registration/...
go test ./events/...
go test ./dynamo/...

# Run a single test
go test -run TestAttemptRegistration ./registration/...
go test -run TestRegisterWithPayment ./registration/...

# Run tests with verbose output
go test -v ./...
```

## Code Style Guidelines

### Imports
- Group imports: stdlib first, then third-party, then internal packages
- Separate groups with blank lines
- Use `goimports` or `gofmt` for formatting

```go
import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"

    "github.com/International-Combat-Archery-Alliance/event-registration/events"
)
```

### Naming Conventions
- **Types**: PascalCase (e.g., `IndividualRegistration`, `EventRepository`)
- **Interfaces**: PascalCase with descriptive names (e.g., `Repository`, `Validator`)
- **Functions**: PascalCase for exported, camelCase for unexported
- **Constants**: UPPER_SNAKE_CASE for error reasons (e.g., `REASON_FAILED_TO_WRITE`)
- **Variables**: camelCase (e.g., `eventID`, `registrationRepo`)
- **Files**: lowercase with underscores avoided (e.g., `registration.go`, `error.go`)

### Error Handling
- Use custom error types with `ErrorReason` constants
- Always wrap errors with context using `fmt.Errorf("...: %w", err)`
- Implement `Unwrap() error` for error chaining
- Check errors using `errors.As()` for type assertions

```go
type ErrorReason string

const (
    REASON_FAILED_TO_WRITE ErrorReason = "FAILED_TO_WRITE"
)

type Error struct {
    Reason  ErrorReason
    Message string
    Cause   error
}

func (e *Error) Error() string {
    return fmt.Sprintf("%s: %s. Cause: %s", e.Reason, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
    return e.Cause
}
```

### Types and Testing
- Use table-driven tests with `t.Run()` for subtests
- Mock external dependencies using struct-based mocks
- Use `testify/assert` for assertions
- Test files should be named `*_test.go`

```go
func TestFunction(t *testing.T) {
    t.Run("description", func(t *testing.T) {
        // test code
        assert.NoError(t, err)
    })
}
```

### Architecture Patterns (Hexagonal/Ports and Adapters)

This codebase follows **Hexagonal Architecture** (Ports and Adapters pattern) to separate business logic from infrastructure concerns:

**Layer Structure:**
```
cmd/           - Entry point, wires dependencies
api/           - Driving adapters (HTTP handlers, middleware)
events/        - Domain: Event aggregate, business logic, repository port
registration/  - Domain: Registration aggregate, business logic, repository port
teams/         - Domain: Team aggregate, business logic, repository port
games/         - Domain: Game aggregate, business logic, repository port
dynamo/        - Driven adapters (DynamoDB repository implementations)
```

**Key Principles:**
- **Ports**: Interfaces defined in domain packages (e.g., `events.Repository`, `registration.Repository`, `payments.CheckoutManager`)
- **Driving Adapters**: `api/` handlers that call domain services
- **Driven Adapters**: `dynamo/` implements repository interfaces; external services (Stripe, AWS SES, auth) implement their ports
- **Dependency Direction**: Domain depends on nothing; infrastructure depends on domain interfaces
- **Dependency Injection**: All dependencies passed via constructors (`NewAPI()`, `NewDB()`)

**Testing Implications:**
- Domain logic tested with mock implementations of ports
- No external dependencies needed for unit tests
- Example: `registration/registration_test.go` mocks `Repository` and `payments.CheckoutManager`

### AWS SAM & Local Development
- Shared infrastructure (DynamoDB, Jaeger, LocalStack) is managed in `icaa.world/docker-compose.yml`
- Start shared infrastructure first: `cd icaa.world && docker compose up -d`
- Use `make local` for full local development environment
- SAM local API gateway runs on port 3000
- Environment variables in `env.json` for local config

### Code Generation
- OpenAPI spec generates server code: `//go:generate go tool oapi-codegen`
- String enums use `go tool stringer`
- Always run `go generate ./...` after modifying specs
