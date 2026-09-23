module github.com/deploycore/deploy-core/apps/api

go 1.24.2

require (
	github.com/jackc/pgx/v5 v5.7.4
	github.com/joho/godotenv v1.5.1
)

require golang.org/x/sys v0.28.0 // indirect

require (
	github.com/deploycore/deploy-core/packages/protocol-go v0.0.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/crypto v0.31.0
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/text v0.21.0 // indirect
)

replace github.com/deploycore/deploy-core/packages/protocol-go => ../../packages/protocol-go
