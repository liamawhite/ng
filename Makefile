ifndef IN_NIX_SHELL
%:
	nix develop --command $(MAKE) $@
else

.PHONY: generate format tidy test lint clean \
	frontend-install frontend-build frontend-dev \
	desktop-build desktop-dev

generate:
	buf generate

format:
	gofmt -w backend/ desktop/
	buf format -w

# go mod tidy -e ignores errors from unpublished workspace-local modules
tidy:
	cd api/golang && go mod tidy
	cd backend    && go mod tidy -e
	cd desktop    && go mod tidy -e

test:
	cd backend && go test -race ./...

lint:
	cd backend && golangci-lint run ./...

clean:
	rm -rf api/golang/*
	rm -rf api/ts/*

frontend-install:
	cd frontend && npm install

frontend-build: frontend-install
	cd frontend && npm run build

frontend-dev: frontend-install
	cd frontend && npm run dev

desktop-build: frontend-build
	cd desktop && wails build

desktop-dev:
	cd desktop && wails dev

endif
