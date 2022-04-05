package client

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	pb "github.com/otamoe/vptun-pb"
	"github.com/spf13/viper"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/tun"
	"google.golang.org/grpc/metadata"
)

type (
	GrpcClient struct {
		Id           string
		RouteSubnet  string
		IRouteSubnet *net.IPNet

		BroadcastIP  string
		IBroadcastIP net.IP

		RouteAddress  string
		IRouteAddress net.IP

		MD metadata.MD

		ConnectAt time.Time

		tunName   string
		tunDevice tun.Device

		grpcHandler         *GrpcHandler
		handlerStreamClient pb.Handler_StreamClient
		mux                 sync.RWMutex
		err                 error
		ctx                 context.Context
		cancel              context.CancelFunc
		close               *atomic.Bool
		closed              chan struct{}

		request chan *pb.StreamRequest
		shells  map[string]*GrpcClientShell

		retry uint64
	}
)

var tunName = 1443

func (grpcClient *GrpcClient) Start() (err error) {
	defer func() {
		grpcClient.logger("stream-end", false, err)
	}()

	// 关闭
	defer func() {
		// 设置成已关闭
		close(grpcClient.closed)

		// 关闭
		if rerr := grpcClient.Close(err); rerr != nil && err == nil {
			err = rerr
		}

		if err == nil && grpcClient.Err() != os.ErrClosed {
			err = grpcClient.Err()
		}
	}()

	// 打开链接
	err = grpcClient.openStream()

	grpcClient.logger("stream-run", false, err)

	if err != nil {
		return
	}

	// 设置网卡名
	if err = grpcClient.setTunName(); err != nil {
		return
	}

	// 打开网卡驱动
	if err = grpcClient.openTunDevice(); err != nil {
		return
	}

	// 运行请求
	go grpcClient.runRequest()

	// 运行响应
	go grpcClient.runResponse()

	// 定时请求 status
	if viper.GetDuration("grpc.status") != 0 {
		go grpcClient.requestStatus()
	}

	// 运行 tun
	go grpcClient.requestTun()

	// 等待
	<-grpcClient.ctx.Done()

	return
}

func (grpcClient *GrpcClient) openStream() (err error) {
	// 打开连接
	if grpcClient.handlerStreamClient, err = grpcClient.grpcHandler.handlerClient.Stream(metadata.NewOutgoingContext(grpcClient.ctx, grpcClient.grpcHandler.md)); err != nil {
		return
	}

	// 读取 header
	if grpcClient.MD, err = grpcClient.handlerStreamClient.Header(); err != nil {
		return
	}

	// 得到 routeAddress
	grpcClient.RouteAddress = strings.Join(grpcClient.MD.Get("client-route-address"), ",")
	if grpcClient.IRouteAddress = net.ParseIP(grpcClient.RouteAddress); grpcClient.IRouteAddress == nil {
		err = fmt.Errorf("error: routing address %s", grpcClient.RouteAddress)
		return
	}
	// 得到 routeSubnet
	grpcClient.RouteSubnet = strings.Join(grpcClient.MD.Get("client-route-subnet"), ",")
	if _, grpcClient.IRouteSubnet, err = net.ParseCIDR(grpcClient.RouteSubnet); err != nil {
		err = fmt.Errorf("error: routing subnet %s %s", grpcClient.RouteSubnet, err)
		return
	}

	// 广播 ip
	grpcClient.IBroadcastIP = make(net.IP, len(grpcClient.IRouteSubnet.IP))
	copy(grpcClient.IBroadcastIP, grpcClient.IRouteSubnet.IP)
	for i, _ := range grpcClient.IRouteSubnet.IP {
		if i >= len(grpcClient.IRouteSubnet.Mask) {
			break
		}
		grpcClient.IBroadcastIP[i] = grpcClient.IBroadcastIP[i] | ^grpcClient.IRouteSubnet.Mask[i]
	}
	grpcClient.BroadcastIP = grpcClient.IBroadcastIP.String()

	return
}

func (grpcClient *GrpcClient) runRequest() (err error) {
	defer func() {
		go grpcClient.Close(err)
	}()
	var request *pb.StreamRequest
	for {
		select {
		case <-grpcClient.ctx.Done():
			return
		case request = <-grpcClient.request:
			if request == nil {
				return
			}
			err = grpcClient.handlerStreamClient.Send(request)

			fields := []zap.Field{}
			if request.Tun != nil {
				fields = append(
					fields,
					zap.Int("requestTunData", len(request.Tun.Data)),
				)
			}
			if request.Shell != nil {
				fields = append(
					fields,
					zap.String("requestShellId", request.Shell.Id),
					zap.String("requestShellData", string(request.Shell.Data)),
					zap.Int32("requestShellStatus", request.Shell.Status),
				)
			}
			if request.Status != nil {
				fields = append(
					fields,
					zap.Bool("requestStatus", true),
				)
				if request.Status.Status != nil && request.Status.Status.Load != nil {
					fields = append(
						fields,
						zap.Float64("requestStatusLoad1", request.Status.Status.Load.Load1),
						zap.Float64("requestStatusLoad5", request.Status.Status.Load.Load5),
						zap.Float64("requestStatusLoad15", request.Status.Status.Load.Load15),
					)
				}
			}
			grpcClient.logger("stream-send", request.Shell == nil, err, fields...)
		}
	}
	return
}

func (grpcClient *GrpcClient) runResponse() (err error) {
	defer func() {
		go grpcClient.Close(err)
	}()
	var response *pb.StreamResponse
	for {
		response, err = grpcClient.handlerStreamClient.Recv()

		fields := []zap.Field{}
		if response != nil {
			if response.Tun != nil {
				fields = append(
					fields,
					zap.Int("responseTunData", len(response.Tun.Data)),
				)
				if err == nil {
					err = grpcClient.OnTun(response.Tun)
				}
			}
			if response.Shell != nil {
				fields = append(
					fields,
					zap.String("responseShellId", response.Shell.Id),
					zap.String("responseShellData", string(response.Shell.Data)),
					zap.Uint32("responseShellTimeout", response.Shell.Timeout),
				)
				if err == nil {
					err = grpcClient.OnShell(response.Shell)
				}
			}
			if response.Status != nil {
				fields = append(
					fields,
					zap.Bool("responseStatus", true),
				)
				if err == nil {
					err = grpcClient.OnStatus(response.Status)
				}
			}
		}

		// log
		grpcClient.logger("stream-recv", response != nil && response.Shell == nil, err, fields...)
		if err != nil {
			return
		}
	}
}

func (grpcClient *GrpcClient) Request(request *pb.StreamRequest) (err error) {
	select {
	case <-grpcClient.ctx.Done():
		// 上下文取消
		err = grpcClient.ctx.Err()
	case <-grpcClient.closed:
		// 已关闭
		err = grpcClient.err
	case grpcClient.request <- request:
		// 写入
		if request.Shell != nil {
			grpcClient.mux.Lock()
			if request.Shell.Status != -1 && len(request.Shell.Data) != 0 {
				delete(grpcClient.shells, request.Shell.Id)
			}
			grpcClient.mux.Unlock()
		}
	}
	return
}

func (grpcClient *GrpcClient) Shell(id string) (grpcClientShell *GrpcClientShell) {
	grpcClient.mux.RLock()
	grpcClientShell, _ = grpcClient.shells[id]
	grpcClient.mux.RUnlock()
	return
}

// 错误
func (grpcClient *GrpcClient) Err() (err error) {
	select {
	case <-grpcClient.closed:
		err = grpcClient.err
		return
	}
}

func (grpcClient *GrpcClient) Close(werr error) (rerr error) {
	if !grpcClient.close.CAS(false, true) {
		rerr = grpcClient.Err()
		return
	}

	// 关闭隧道
	if grpcClient.tunDevice != nil {
		grpcClient.tunDevice.Flush()
		if rerr2 := grpcClient.tunDevice.Close(); rerr2 != nil && rerr == nil {
			rerr = rerr2
		}

		// 删除路由表
		{
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			cmd := exec.CommandContext(ctx, "route", "delete", "-host", "10.128.0.0/9")
			cmd.Run()
		}
	}

	if werr == nil {
		werr = os.ErrClosed
	}

	shells := []*GrpcClientShell{}
	grpcClient.mux.RLock()
	for _, shell := range grpcClient.shells {
		shells = append(shells, shell)
	}
	grpcClient.mux.RUnlock()

	for _, shell := range shells {
		if rerr2 := shell.Close(1, context.Canceled); rerr2 != nil && rerr2 != os.ErrClosed && rerr == nil {
			rerr = rerr2
		}
	}
	grpcClient.err = werr

	// 申请关闭
	grpcClient.cancel()

	// 连接已关闭
	<-grpcClient.closed

	if grpcClient.handlerStreamClient != nil {
		if rerr2 := grpcClient.handlerStreamClient.CloseSend(); rerr2 != nil && rerr == nil {
			rerr = rerr2
		}
	}

	return
}

func (grpcClient *GrpcClient) logger(name string, debug bool, err error, fields ...zap.Field) {
	fields = append(
		fields, zap.Duration("duration", time.Now().Sub(grpcClient.ConnectAt)),
	)
	if grpcClient.RouteAddress != "" {
		fields = append(
			fields,
			zap.String("routeAddress", grpcClient.RouteAddress),
		)
	}
	if grpcClient.RouteSubnet != "" {
		fields = append(
			fields,
			zap.String("routeSubnet", grpcClient.RouteSubnet),
		)
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		if IsErrClose(err) {
			grpcClient.grpcHandler.logger.Info(name, fields...)
		} else {
			fields = append(fields, zap.Stack("stack"))
			grpcClient.grpcHandler.logger.Error(name, fields...)
		}
	} else {
		if debug {
			grpcClient.grpcHandler.logger.Debug(name, fields...)
		} else {
			grpcClient.grpcHandler.logger.Info(name, fields...)
		}
	}
}
