package client

import (
	"context"
	"errors"
	"os"
	"path"
	"sync"
	"time"

	liblogger "github.com/otamoe/go-library/logger"
	pb "github.com/otamoe/vptun-pb"
	"github.com/spf13/viper"
	"go.uber.org/atomic"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc/metadata"
)

type (
	GrpcHandler struct {
		logger        *zap.Logger
		handlerClient pb.HandlerClient
		ctx           context.Context
		cancel        context.CancelFunc
		waitGroup     sync.WaitGroup
		clientId      string
		clientKey     string
		md            metadata.MD
		configPath    string

		userHomeDir string
		hostname    string
	}
)

func NewGrpcHandler(ctx context.Context, handlerClient pb.HandlerClient, lc fx.Lifecycle) (grpcHandler *GrpcHandler, err error) {
	// 用户主页
	var userHomeDir string
	if userHomeDir, err = os.UserHomeDir(); err != nil {
		return
	}
	grpcHandler = &GrpcHandler{
		configPath:    path.Join(userHomeDir, ".vptun/client/grpc/config.yaml"),
		logger:        liblogger.Get("grpc"),
		handlerClient: handlerClient,
		clientId:      viper.GetString("grpc.clientId"),
		clientKey:     viper.GetString("grpc.clientKey"),
		userHomeDir:   userHomeDir,
	}
	grpcHandler.ctx, grpcHandler.cancel = context.WithCancel(ctx)
	grpcHandler.hostname, _ = os.Hostname()
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return grpcHandler.start()
		},
		OnStop: func(ctx context.Context) error {
			grpcHandler.cancel()
			grpcHandler.waitGroup.Wait()
			return nil
		},
	})
	return
}

func (grpcHandler *GrpcHandler) start() (err error) {
	// client id 不存在 自动创建
	if grpcHandler.clientId == "" && grpcHandler.clientKey == "" && !grpcHandler.configExist() {
		if err = grpcHandler.create(); err != nil {
			return
		}
	}

	if grpcHandler.clientId == "" {
		err = errors.New("Client id not found")
		return
	}

	if grpcHandler.clientKey == "" {
		err = errors.New("Client key not found")
		return
	}

	grpcHandler.md = metadata.New(map[string]string{
		"client-hostname": grpcHandler.hostname,
		"client-id":       grpcHandler.clientId,
		"client-key":      grpcHandler.clientKey,
	})
	if viper.GetBool("grpc.shell") {
		grpcHandler.md.Append("client-shell", "true")
	}
	grpcHandler.waitGroup.Add(1)
	go grpcHandler.startStream()
	return nil
}

func (grpcHandler *GrpcHandler) create() (err error) {
	var response *pb.CreateResponse

	defer func() {
		fields := []zap.Field{
			zap.String("clientHostname", grpcHandler.hostname),
		}
		if response != nil && response.Client != nil {
			fields = append(
				fields,
				zap.String("clientId", response.Client.Id),
				zap.String("connectAddress", response.Client.ConnectAddress),
			)
		}
		if err == nil {
			grpcHandler.logger.Info(
				"create",
				fields...,
			)
		} else {
			fields = append(fields, zap.Error(err))
			grpcHandler.logger.Error(
				"create",
				fields...,
			)
		}
	}()

	if response, err = grpcHandler.handlerClient.Create(
		metadata.NewOutgoingContext(grpcHandler.ctx, metadata.New(map[string]string{"client-hostname": grpcHandler.hostname})),
		&pb.CreateRequest{},
	); err != nil {
		return
	}

	grpcHandler.clientId = response.Client.Id
	grpcHandler.clientKey = response.Client.Key

	v := viper.New()
	v.Set("grpc.clientId", response.Client.Id)
	v.Set("grpc.clientKey", response.Client.Key)
	v.SetConfigType("yaml")
	if err = v.WriteConfigAs(grpcHandler.configPath); err != nil {
		return
	}
	return
}

func (grpcHandler *GrpcHandler) startStream() {
	defer grpcHandler.waitGroup.Done()
	defer grpcHandler.cancel()
	var retry uint64 = 0
	for {
		var t *time.Timer
		if retry == 0 {
			t = time.NewTimer(time.Second)
		} else {
			t = time.NewTimer(viper.GetDuration("grpc.retryInterval"))
		}
		select {
		case <-grpcHandler.ctx.Done():
			t.Stop()
			return
		case <-t.C:
			grpcClient := &GrpcClient{
				retry:       retry,
				grpcHandler: grpcHandler,
				ConnectAt:   time.Now(),
				close:       atomic.NewBool(false),
				closed:      make(chan struct{}),
				request:     make(chan *pb.StreamRequest, 4),
				shells:      map[string]*GrpcClientShell{},
			}
			grpcClient.ctx, grpcClient.cancel = context.WithCancel(grpcHandler.ctx)
			grpcClient.Start()
		}
		retry++
	}
}

func (grpcHandler *GrpcHandler) configExist() bool {
	_, err := os.Stat(grpcHandler.configPath)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err)
}
