.PHONY: compile
compile: ## Compile the proto file.
	protoc -I pkg/mafiapb/ pkg/mafiapb/mafia.proto --go_out=plugins=grpc:pkg/mafiapb/

.PHONY: server
server: ## Build and run server.
	go build -o bin/server server/main.go
	bin/server

.PHONY: client
client: ## Build and run client.
	go build -o bin/client client/main.go
	bin/client
