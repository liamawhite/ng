package server

import (
	"context"
	"fmt"

	protovalidate "buf.build/go/protovalidate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var validator protovalidate.Validator

func init() {
	var err error
	validator, err = protovalidate.New()
	if err != nil {
		panic(fmt.Sprintf("init protovalidate: %v", err))
	}
}

// ValidationInterceptor is a gRPC unary server interceptor that validates
// incoming requests against protovalidate field constraints declared in the
// proto definitions (e.g. min_len, required).
//
// NOTE: This interceptor only fires when requests arrive over the gRPC
// transport. The HTTP gateway uses RegisterXxxHandlerClient which dials the
// gRPC server over loopback, so HTTP requests DO pass through this interceptor
// — but at the cost of an extra loopback TCP round-trip per HTTP request. For
// a local tool this is negligible; for high-throughput services consider
// RegisterXxxHandlerServer (direct call, no hop) and validate separately.
func ValidationInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if msg, ok := req.(proto.Message); ok {
		if err := validator.Validate(msg); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	return handler(ctx, req)
}
