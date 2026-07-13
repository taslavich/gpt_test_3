package utils

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func WrapError(prefix string, wrapError error) error {
	if wrapError == nil {
		return status.Errorf(codes.Unknown, "%s: unknown error", prefix)
	}
	if st, ok := status.FromError(wrapError); ok {
		return status.Errorf(st.Code(), "%s: %s", prefix, st.Message())
	}
	return status.Errorf(codes.Unknown, "%s: %s", prefix, wrapError.Error())
}
