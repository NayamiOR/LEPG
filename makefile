APP_NAME = LEPG
BIN_DIR = bin

ifdef OS
   # Windows (Windows_NT)
	RM = rm -f
	CP = copy /Y
	MKDIR = mkdir
	# Windows 下路径分隔符必须是反斜杠，且 .exe 后缀
	FixPath = $(subst /,\,$1)
	EXEC_EXT = .exe
	# Windows mkdir 如果目录存在会报错，需要忽略错误
	MKDIR_P = -mkdir
else
	# Linux / Unix / macOS
	RM = rm -f
	CP = cp -f
	MKDIR = mkdir -p
	# Linux 保持原样
	FixPath = $1
	EXEC_EXT =
	MKDIR_P = mkdir -p
endif

run:

run-client:
	go run $(call FixPath, ./cmd/client/main.go) run

run-server:
	go run $(call FixPath, ./cmd/server/main.go) run

c: run-client

s: run-server

simmodbus:
	go run $(call FixPath, ./cmd/modbus-sim)

simmqtt:
	go run $(call FixPath, ./cmd/mqtt-sim)

clean:
	$(RM) $(call FixPath,$(BIN_DIR)/*)

build:
	go build -o $(call FixPath,$(BIN_DIR)/lepgc$(EXEC_EXT)) $(call FixPath, ./cmd/client)
	go build -o $(call FixPath,$(BIN_DIR)/lepgs$(EXEC_EXT)) $(call FixPath, ./cmd/server)

echo:
	echo "Hello"

# 测试相关命令
test:
	go test -v ./...

test-config:
	go test -v ./internal/config/...

test-coverage:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html

test-race:
	go test -race -v ./...

# 帮助信息
help:
	@echo "Available targets:"
	@echo "  build          - Build the application"
	@echo "  build-dev      - Build with race detection (dev mode)"
	@echo "  build-all      - Build for all platforms"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  clean          - Clean build artifacts"
	@echo "  version        - Show version information"
	@echo "  help           - Show this help message"
