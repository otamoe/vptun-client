package client

import (
	"errors"

	libgrpc "github.com/otamoe/go-library/grpc"
	liblogger "github.com/otamoe/go-library/logger"
	"go.uber.org/fx"
)

const UserAgent = "vptun-client/0.1.0"

var ErrClientCA = errors.New("Client CA")
var ErrClientCertificate = errors.New("Client certificate")

var logger = liblogger.Get("client")

func New() fx.Option {
	return fx.Options(
		libgrpc.NewClient(),
		fx.Provide(GrpcTargetAddressOption),
		fx.Provide(GrpcUserAgentOption),
		fx.Provide(GrpcKeepaliveParamsOption),
		fx.Provide(GrpcTransportCredentialsOption),

		fx.Provide(GrpcRegisterHandler),

		fx.Invoke(NewGrpcHandler),
	)
}
