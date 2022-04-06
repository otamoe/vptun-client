package client

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	pb "github.com/otamoe/vptun-pb"
	sutilNet "github.com/shirou/gopsutil/v3/net"
	"go.uber.org/zap"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

func (grpcClient *GrpcClient) setTunName() (err error) {
	if interfaceStatList, _ := sutilNet.InterfacesWithContext(grpcClient.ctx); len(interfaceStatList) != 0 {
		// 读取到了网卡信息 存在才递增
		exists := map[string]bool{}
		for _, interfaceStat := range interfaceStatList {
			exists[interfaceStat.Name] = true
		}
		for i := 0; i < 10; i++ {
			if _, ok := exists[fmt.Sprintf("utun%d", tunName)]; ok {
				tunName++
			}
		}
	} else {
		// 没读到网卡信息自动递增
		if grpcClient.retry != 0 {
			tunName++
		}

	}

	// 大于 超过6位数
	if tunName > 99999 {
		tunName = 1443
	}
	grpcClient.tunName = fmt.Sprintf("utun%d", tunName)
	return
}

func (grpcClient *GrpcClient) runCmd(ctx context.Context, name string, args ...string) (err error) {
	// 配置 网卡
	cmd := exec.CommandContext(ctx, name, args...)
	stderr := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()

	fields := []zap.Field{
		zap.String("name", name),
		zap.Strings("args", args),
		zap.String("stdout", stdout.String()),
		zap.String("stderr", stderr.String()),
	}
	grpcClient.logger("cmd", false, err, fields...)

	if err != nil {
		err = fmt.Errorf("error: %s %v %s", name, args, err)
		return
	}
	return
}
func (grpcClient *GrpcClient) openTunDevice() (err error) {

	// 打开 tunDevice
	if grpcClient.tunDevice, err = tun.CreateTUN(grpcClient.tunName, device.DefaultMTU); err != nil {
		if strings.Index(err.Error(), "dev/net/tun") != -1 {
			// tun 未启用 启用模块
			grpcClient.runCmd(grpcClient.ctx, "modprobe", "tun")

			// 再次打开
			grpcClient.tunDevice, err = tun.CreateTUN(grpcClient.tunName, device.DefaultMTU)
		}
		if err != nil {
			err = fmt.Errorf("error: CreateTUN(%s): %s", grpcClient.tunName, err)
			return
		}
	}

	// 配置 网卡
	if err = grpcClient.runCmd(grpcClient.ctx, "ifconfig", grpcClient.tunName, grpcClient.IRouteAddress.String(), "netmask", "0x"+grpcClient.IRouteSubnet.Mask.String(), "broadcast", grpcClient.IBroadcastIP.String(), "up"); err != nil {
		return
	}

	// 配置路由表
	if runtime.GOOS == "darwin" {
		if err = grpcClient.runCmd(grpcClient.ctx, "route", "add", "-net", grpcClient.IRouteSubnet.String(), "-interface", grpcClient.tunName); err != nil {
			return
		}
	} else {
		if err = grpcClient.runCmd(grpcClient.ctx, "route", "add", "-net", grpcClient.IRouteSubnet.String(), "gw", grpcClient.IRouteAddress.String(), grpcClient.tunName); err != nil {
			return
		}
	}
	return

}

func (grpcClient *GrpcClient) OnTun(tunResponse *pb.TunResponse) (err error) {
	if len(tunResponse.Data) == 0 {
		return
	}
	b := make([]byte, len(tunResponse.Data)+4)
	copy(b[4:], tunResponse.Data)
	if _, err = grpcClient.tunDevice.Write(b, 4); err != nil {
		return
	}
	return
}

func (grpcClient *GrpcClient) requestTun() (err error) {
	defer func() {
		go grpcClient.Close(err)
	}()
	var n int
	data := make([]byte, device.DefaultMTU*2)
	for {
		if n, err = grpcClient.tunDevice.Read(data, 4); err != nil {
			return
		}
		b := make([]byte, n+4)
		copy(b, data)

		if b[4]>>4 == ipv6.Version {
			// ipv6
			var header *ipv6.Header
			if header, err = ipv6.ParseHeader(b[4:]); err != nil {
				err = fmt.Errorf("parse Header", err)
				return
			}
			// 本地
			if header.Src.Equal(header.Dst) {
				if _, err = grpcClient.tunDevice.Write(b, 4); err != nil {
					return
				}
				continue
			}
			// 远程
			if err = grpcClient.Request(&pb.StreamRequest{Tun: &pb.TunRequest{Data: b[4:]}}); err != nil {
				return
			}
		} else {
			// ipv4
			var header *ipv4.Header
			if header, err = ipv4.ParseHeader(b[4:]); err != nil {
				err = fmt.Errorf("parse Header", err)
				return
			}

			// 本地
			if header.Src.Equal(header.Dst) {
				if _, err = grpcClient.tunDevice.Write(b, 4); err != nil {
					return
				}
				continue
			}

			// 远程
			if err = grpcClient.Request(&pb.StreamRequest{Tun: &pb.TunRequest{Data: b[4:]}}); err != nil {
				return
			}
		}
	}
}
