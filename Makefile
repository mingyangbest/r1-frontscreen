VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build clean web test-touch test vet fmt

build:
	@mkdir -p build
	CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o build/r1-toolbox ./cmd
	@echo "✓ build/r1-toolbox $(VERSION) ($(COMMIT))"

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l internal cmd

test-touch:
	@mkdir -p build
	@echo "构建触摸屏测试程序..."
	CGO_ENABLED=0 go build -ldflags="-s -w" -o build/test-touch ./cmd/test_touch
	@echo "✓ 测试程序已构建到 build/test-touch"

web:
	@echo "构建 Web UI..."
	@cd web-ui && npm install && npm run build
	@echo "✓ Web UI 已构建到 internal/web/dist/"

clean:
	@rm -rf build
	@rm -rf internal/web/dist
