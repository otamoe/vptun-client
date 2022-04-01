package client

import (
	"context"
	"os"
	"os/exec"
	"time"

	pb "github.com/otamoe/vptun-pb"
	"github.com/spf13/viper"
	"go.uber.org/atomic"
	"go.uber.org/zap"
)

type (
	GrpcClientShell struct {
		ctx    context.Context
		cancel context.CancelFunc
		close  *atomic.Bool
		closed chan struct{}

		*exec.Cmd
		Id         string
		grpcClient *GrpcClient
		writeSize  int
	}
)

func (grpcClient *GrpcClient) OnShell(shellResponse *pb.ShellResponse) (err error) {
	if shellResponse.Id == "" {
		return
	}
	if !viper.GetBool("grpc.shell") {
		err = grpcClient.Request(&pb.StreamRequest{Shell: &pb.ShellRequest{Id: shellResponse.Id, Data: []byte("Shell not support"), Status: 1}})
		return
	}

	grpcClient.mux.RLock()
	grpcClientShell, _ := grpcClient.shells[shellResponse.Id]
	grpcClient.mux.RUnlock()

	// 取消
	if len(shellResponse.Data) == 0 || shellResponse.Timeout == 0 {
		if grpcClientShell == nil {
			err = grpcClient.Request(&pb.StreamRequest{Shell: &pb.ShellRequest{Id: shellResponse.Id, Data: []byte("Shell not found"), Status: 1}})
			return
		}
		err = grpcClientShell.Close(1, context.Canceled)
		if err == os.ErrClosed {
			err = nil
		}
		return
	}

	// 已存在直接返回
	if grpcClientShell != nil {
		return
	}

	// 设置 grpcClientShell
	grpcClientShell = &GrpcClientShell{
		Id:         shellResponse.Id,
		close:      atomic.NewBool(false),
		closed:     make(chan struct{}),
		grpcClient: grpcClient,
	}
	if shellResponse.Timeout > 3600*6 {
		shellResponse.Timeout = 3600 * 6
	}

	fields := []zap.Field{
		zap.String("shellId", grpcClientShell.Id),
		zap.String("data", string(shellResponse.Data)),
		zap.Uint32("timeout", shellResponse.Timeout),
	}
	grpcClientShell.grpcClient.logger("shell-open", false, nil, fields...)

	grpcClientShell.ctx, grpcClientShell.cancel = context.WithTimeout(grpcClient.ctx, time.Second*time.Duration(shellResponse.Timeout))
	grpcClientShell.Cmd = exec.CommandContext(grpcClientShell.ctx, "sh", "-c", string(shellResponse.Data))
	grpcClientShell.Dir = grpcClient.grpcHandler.userHomeDir
	grpcClientShell.Stderr = grpcClientShell
	grpcClientShell.Stdout = grpcClientShell

	grpcClient.mux.Lock()
	grpcClient.shells[grpcClientShell.Id] = grpcClientShell
	grpcClient.mux.Unlock()

	go func() {
		var err error

		defer grpcClientShell.cancel()
		defer close(grpcClientShell.closed)

		// 运行
		err = grpcClientShell.Run()

		// 状态
		var status int32
		if err != nil {
			if grpcClientShell.ProcessState != nil {
				status = int32(grpcClientShell.ProcessState.ExitCode())
			}
		}
		// 关闭
		go grpcClientShell.Close(status, err)
	}()
	return
}

func (grpcClientShell *GrpcClientShell) Write(p []byte) (n int, err error) {
	err = grpcClientShell.grpcClient.Request(&pb.StreamRequest{
		Shell: &pb.ShellRequest{
			Id:     grpcClientShell.Id,
			Data:   p,
			Status: -1,
		}})
	if err != nil {
		return
	}
	n = len(p)
	grpcClientShell.writeSize += n
	return
}

func (grpcClientShell *GrpcClientShell) Close(status int32, werr error) (rerr error) {
	grpcClientShell.grpcClient.mux.Lock()
	delete(grpcClientShell.grpcClient.shells, grpcClientShell.Id)
	grpcClientShell.grpcClient.mux.Unlock()

	// 已执行过了
	if !grpcClientShell.close.CAS(false, true) {
		return
	}

	grpcClientShell.cancel()

	var data []byte
	if werr != nil {
		if grpcClientShell.writeSize != 0 {
			data = []byte(werr.Error())
		} else {
			data = []byte("\n" + werr.Error())
		}
	}

	rerr = grpcClientShell.grpcClient.Request(&pb.StreamRequest{
		Shell: &pb.ShellRequest{
			Id:     grpcClientShell.Id,
			Data:   data,
			Status: status,
		},
	})

	fields := []zap.Field{
		zap.String("shellId", grpcClientShell.Id),
		zap.Int32("status", status),
	}

	if rerr != nil {
		grpcClientShell.grpcClient.logger("shell-close", false, rerr, fields...)
	} else {
		grpcClientShell.grpcClient.logger("shell-close", false, werr, fields...)
	}
	<-grpcClientShell.closed
	return
}
