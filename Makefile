SHELL := /bin/sh
VERSION ?= 1.0.0
COMPOSE ?= docker compose
BINARY := bin/tingfeng
.PHONY: help web build run test verify dev-api dev-web docker-build up down logs status setup-token reset-password backup
help:
	@echo "make up          构建并启动 Docker 服务"
	@echo "make setup-token 读取首次初始化令牌（仅终端显示）"
	@echo "make build       构建 Vue + Go 单文件程序"
	@echo "make run         构建并本地运行"
	@echo "make test        Go 竞态测试 + 前端逻辑检查"
	@echo "make verify      完整测试、静态检查和生产构建"
	@echo "make backup      创建包含数据库与密钥的完整备份"
web:
	cd frontend && npm ci && npm run build
build: web
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/tingfeng
run: build
	./$(BINARY) serve
test:
	go test -race ./...
	cd frontend && npm test
verify: test build
	go vet ./...
dev-api:
	go run ./cmd/tingfeng
dev-web:
	cd frontend && npm ci && npm run dev
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t tingfeng:local .
up:
	$(COMPOSE) up -d --build
down:
	$(COMPOSE) down
logs:
	$(COMPOSE) logs --tail=100 -f
status:
	$(COMPOSE) ps
setup-token:
	@$(COMPOSE) cp tingfeng:/data/setup.token - | tar -xO
	@printf '\n'
reset-password:
	$(COMPOSE) exec tingfeng /tingfeng reset-password --confirm
backup:
	@mkdir -p backups
	@stamp=$$(date +%Y%m%d-%H%M%S); \
	$(COMPOSE) stop || exit 1; \
	trap '$(COMPOSE) start >/dev/null' EXIT; \
	$(COMPOSE) cp tingfeng:/data backups/$$stamp && \
	echo "已备份到 backups/$$stamp。包含密钥，请私密保管。"
