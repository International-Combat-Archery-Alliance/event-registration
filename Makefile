.PHONY: build
build:
	go generate ./...
	sam build

.PHONY: local
local: build
	docker run --privileged --rm tonistiigi/binfmt --install all
	docker-compose up -d
	sam local start-api
