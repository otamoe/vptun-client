package client

import (
	"fmt"
	"os/exec"

	pb "github.com/otamoe/vptun-pb"
	sutilNet "github.com/shirou/gopsutil/v3/net"
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

func (grpcClient *GrpcClient) openTunDevice() (err error) {

	// 打开 tunDevice
	if grpcClient.tunDevice, err = tun.CreateTUN(grpcClient.tunName, device.DefaultMTU); err != nil {
		err = fmt.Errorf("error: CreateTUN(%s): %s", grpcClient.tunName, err)
		return
	}

	// 配置 网卡
	cmd := exec.CommandContext(grpcClient.ctx, "ifconfig", grpcClient.tunName, grpcClient.IRouteAddress.String(), "netmask", "0x"+grpcClient.IRouteSubnet.Mask.String(), "broadcast", grpcClient.IBroadcastIP.String(), "up")
	if err = cmd.Run(); err != nil {
		return
	}

	// 配置路由表
	cmd2 := exec.CommandContext(grpcClient.ctx, "route", "add", "-net", grpcClient.IRouteSubnet.String(), "-interface", grpcClient.tunName)
	if err = cmd2.Run(); err != nil {
		return
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
	packet := make([]byte, device.DefaultMTU*2)
	for {
		if n, err = grpcClient.tunDevice.Read(packet, 4); err != nil {
			return
		}
		b := make([]byte, n+4)
		copy(b, packet)

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
