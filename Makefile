.PHONY: armSupport
armSupport:
	docker run --privileged --rm tonistiigi/binfmt --install all

.PHONY: build
build: armSupport
	go generate ./...
	sam build

.PHONY: local
local: build
	docker-compose up -d
	sam local start-api
