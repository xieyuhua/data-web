.PHONY: dev-api dev-web build test tidy clean

dev-api:
	go run -tags dev ./cmd/server

dev-web:
	cd web && npm run dev

build:
	cd web && npm install && npm run build
	go build -trimpath -ldflags="-s -w" -o bi.exe ./cmd/server

test:
	go test ./...

tidy:
	GOPROXY=https://proxy.golang.org,direct go mod tidy

clean:
	rm -f bi.exe
	rm -rf internal/webui/dist/assets
