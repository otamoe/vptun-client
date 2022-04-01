package client

import (
	"time"

	pb "github.com/otamoe/vptun-pb"
	"github.com/spf13/viper"
)

func (grpcClient *GrpcClient) requestStatus() (err error) {
	if viper.GetDuration("grpc.status") == 0 {
		return
	}
	var status *pb.Status
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
				t.Reset(viper.GetDuration("grpc.status"))
				if status, err = GetStatus(grpcClient.ctx); err == nil {
					err = grpcClient.Request(&pb.StreamRequest{Status: &pb.StatusRequest{Status: status}})
				}
				if err != nil {
					return
				}
			}
		}
	}
	return
}

func (grpcClient *GrpcClient) OnStatus(statusResponse *pb.StatusResponse) (err error) {
	return
}
