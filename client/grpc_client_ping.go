package client

import (
	"time"

	pb "github.com/otamoe/vptun-pb"
	"github.com/spf13/viper"
)

func (grpcClient *GrpcClient) requestPing() (err error) {
	pingD := viper.GetDuration("grpc.time")
	if pingD < time.Second*10 {
		pingD = time.Second * 10
	}
	pingD = pingD - (time.Second * 5)
	defer func() {
		go grpcClient.Close(err)
	}()
	for {
		t := time.NewTicker(time.Second * 3)
		defer t.Stop()
		for {
			select {
			case <-grpcClient.ctx.Done():
				return
			case <-grpcClient.closed:
				return
			case <-t.C:
				// 读取状态
				t.Reset(pingD)
				err = grpcClient.Request(&pb.StreamRequest{Ping: &pb.PingRequest{Pong: false}})
				if err != nil {
					return
				}
			}
		}
	}
	return
}

func (grpcClient *GrpcClient) OnPing(pingResponse *pb.PingResponse) (err error) {
	if !pingResponse.Pong {
		err = grpcClient.Request(&pb.StreamRequest{Ping: &pb.PingRequest{Pong: true}})
	}
	return
}
