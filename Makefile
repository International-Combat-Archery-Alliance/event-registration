.PHONY: build
build: 
	go generate ./...
	sam build --parameter-overrides architecture=x86_64

.PHONY: local
local: build
	docker-compose up -d
	sam local start-api --docker-network icaa-event-registration --parameter-overrides architecture=x86_64 --warm-containers EAGER
