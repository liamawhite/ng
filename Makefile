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
	rm -f api/golang/ng.pb.go api/golang/ng_grpc.pb.go
	rm -rf api/ts/*

endif
