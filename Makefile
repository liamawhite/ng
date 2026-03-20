ifndef IN_NIX_SHELL
%:
	nix develop --command $(MAKE) $@
else

.PHONY: generate format tidy test lint clean

generate:
	buf generate

format:
	gofmt -w backend/
	buf format -w

# go mod tidy -e ignores errors from unpublished workspace-local modules
tidy:
	cd api/golang && go mod tidy
	cd backend    && go mod tidy -e

test:
	cd backend && go test -race ./...

lint:
	cd backend && golangci-lint run ./...

clean:
	rm -rf api/golang/*
	rm -rf api/ts/*

endif
