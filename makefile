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

run: clean build

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

