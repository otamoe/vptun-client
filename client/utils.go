package client

import (
	"context"
	"errors"
	"io"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func IsErrClose(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err.Error() == io.EOF.Error() {
		return true
	}
	if err == os.ErrClosed || err.Error() == os.ErrClosed.Error() {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if errors.Is(err, grpc.ErrClientConnClosing) {
		return true
	}

	if code := status.Code(err); code == codes.Canceled || code == codes.DeadlineExceeded {
		return true
	}

	return false
}
