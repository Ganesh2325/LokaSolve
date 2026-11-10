.PHONY: infra test test-integration test-ai web-lint

infra:
	docker compose up -d postgres redis minio minio-init

test:
	cd services/api-go && go test ./...

test-integration:
	cd services/api-go && go test -tags integration ./...

test-ai:
	cd services/ai-python && python -m pytest

web-lint:
	cd apps/web && npx tsc --noEmit
