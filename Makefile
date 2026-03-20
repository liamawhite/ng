ifndef IN_NIX_SHELL
%:
	nix develop --command $(MAKE) $@
else

.PHONY: generate format tidy clean

generate:
	buf generate

format:
	gofmt -w backend/
	buf format -w

# go mod tidy -e ignores errors from unpublished workspace-local modules
tidy:
	cd api/golang && go mod tidy
	cd backend    && go mod tidy -e

clean:
	rm -rf api/golang/*
	rm -rf api/ts/*

endif
