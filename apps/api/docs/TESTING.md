# Test strategy (B33)

Behavior-focused tests for the control plane. Prefer observable outcomes
(HTTP status/codes, DB state, deployment status) over private implementation
details.

## Unit

| Area | Location |
| --- | --- |
| Deployment state machine | `internal/deployments/statemachine_test.go` |
| Rollback eligibility | `internal/deployments/rollback_test.go` |
| Placement | `internal/placement/placement_test.go` |
| Secret encryption | `pkg/crypto/aead_test.go` |
| RBAC authorizer | `internal/rbac/authorizer_test.go` |
| Revision numbering | `internal/orchestrator` (`TestOrchestratorRevisionNumbersIncrement`) |

## Repository integration

| Area | Location |
| --- | --- |
| Tenant isolation | `internal/deployments/repository_integration_test.go` (`TestListIsolatesByOrganization`) |
| Transactions / optimistic concurrency | same (`TestTransitionInvalidDoesNotMutate`, `TestTransitionOptimisticConflict`) |
| Constraints | same (idempotency key + revision number uniqueness) |

## HTTP

| Area | Location |
| --- | --- |
| Auth | `internal/auth/auth_integration_test.go` (+ `TestProtectedRouteRequiresBearer`) |
| Authorization | org/resource integration suites (viewer 403 / owner OK) |
| Validation / errors | `TestDeploymentHTTPValidationAndErrorCodes`, `pkg/apierror` |

## Deployment paths

| Path | Test |
| --- | --- |
| Success | `TestOrchestratorHappyPathSimulated`, `TestOrchestratorViaJobWorker` |
| Build failure | `TestOrchestratorBuildFailure` (`ForceBuildFail`) |
| Start failure | `TestOrchestratorStartFailure` (`ForceStartFail`) |
| Health failure | forward + rollback preserve-active tests (`ForceHealthFail`) |
| Cancellation | `TestDeploymentCreateQueueCancelIdempotency` |
| Timeout | `TestOrchestratorPreservesPreviousRevisionOnFailure` |
| Concurrent transition | stale `Advance` / `Transition` → conflict |

## Running

```bash
# Unit + packages that skip without Postgres:
go test ./...

# Or point at a dedicated DB:
export TEST_DATABASE_URL='postgres://localhost/deploycore_test?sslmode=disable'
go test ./...
```
