# Aplicación de escritorio — UPB-CIENTÍFICA
#
# Fyne usa OpenGL a través de cgo, así que cada sistema se compila en su
# propio sistema: el binario de Windows se hace en Windows y el de Linux en
# Linux. No hay contenedores de por medio.

BIN := bin/upb-escritorio

.PHONY: build run test proto limpiar

build:
	go build -o $(BIN) ./cmd/upb-escritorio

run: build
	./$(BIN)

test:
	go test ./...

# Regenera los stubs de gRPC tras traer el .proto de file-sync.
# Necesita protoc, protoc-gen-go y protoc-gen-go-grpc.
proto:
	protoc --go_out=. --go_opt=module=github.com/upb-cientifica/desktop-app \
	       --go-grpc_out=. --go-grpc_opt=module=github.com/upb-cientifica/desktop-app \
	       proto/filesync/v1/filesync.proto

limpiar:
	rm -rf bin
