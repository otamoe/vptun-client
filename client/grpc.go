package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"os"
	"path"
	"time"

	libgrpc "github.com/otamoe/go-library/grpc"
	libviper "github.com/otamoe/go-library/viper"
	pb "github.com/otamoe/vptun-pb"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	configDir := path.Join(userHomeDir, ".vptun")

	libviper.SetDefault("grpc.targetAddress", "127.0.0.1:9443", "Dial address")
	libviper.SetDefault("grpc.dialTimeout", time.Second*60, "Dial timeout")
	libviper.SetDefault("grpc.retryInterval", time.Minute, "Retry interval")
	libviper.SetDefault("grpc.time", time.Minute*3, "Ping interval")
	libviper.SetDefault("grpc.timeout", time.Second*30, "Ping timeout")

	libviper.SetDefault("grpc.tlsCA", path.Join(configDir, "client/grpc/ca.crt"), "Certification authority")
	libviper.SetDefault("grpc.tlsCrt", path.Join(configDir, "client/grpc/client.crt"), "Credentials")
	libviper.SetDefault("grpc.tlsKey", path.Join(configDir, "client/grpc/client.key"), "Private key")

	libviper.SetDefault("grpc.clientId", "", "Client id")
	libviper.SetDefault("grpc.clientKey", "", "Client key")

	libviper.SetDefault("grpc.shell", false, "Execute shell commands")
	libviper.SetDefault("grpc.status", time.Duration(0), "Upload status interval")

}

func GrpcTargetAddressOption() (out libgrpc.OutExtendedDialOption) {
	out.Option = func(extendedDialOptions *libgrpc.ExtendedDialOptions) (err error) {
		extendedDialOptions.TargetAddress = viper.GetString("grpc.targetAddress")
		return nil
	}
	return
}

func GrpcUserAgentOption() (out libgrpc.OutDialOption) {
	out.Option = grpc.WithUserAgent(UserAgent)
	return
}

func GrpcKeepaliveParamsOption() (out libgrpc.OutDialOption) {
	out.Option = grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                viper.GetDuration("grpc.time"),
		Timeout:             viper.GetDuration("grpc.timeout"),
		PermitWithoutStream: true,
	})
	return
}

func GrpcTransportCredentialsOption() (out libgrpc.OutDialOption, err error) {
	var tlsConfig *tls.Config
	if tlsConfig, err = parseTLS(viper.GetString("grpc.tlsCA"), viper.GetString("grpc.tlsCrt"), viper.GetString("grpc.tlsKey")); err != nil {
		return
	}
	// tls 配置
	tlsConfig.ServerName = "server"
	out.Option = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	return
}

func GrpcRegisterHandler(ctx context.Context, clientConn *grpc.ClientConn) (handlerClient pb.HandlerClient) {
	handlerClient = pb.NewHandlerClient(clientConn)
	return
}

func parseTLS(ca string, crt string, key string) (tlsConfig *tls.Config, err error) {
	// tls 配置
	tlsConfig = &tls.Config{
		MinVersion: tls.VersionTLS13, // 最低版本 1.3
	}

	// server ca
	if ca != "" {
		certPool := x509.NewCertPool()
		var b []byte
		if b, err = ioutil.ReadFile(ca); err != nil {
			b = []byte(ca)
		}
		if !certPool.AppendCertsFromPEM(b) {
			err = ErrClientCA
			return
		}
		err = nil

		// root ca
		tlsConfig.RootCAs = certPool
	}

	// server crt, key
	var certificate tls.Certificate
	if certificate, err = tls.LoadX509KeyPair(crt, key); err != nil {
		if certificate, err = tls.X509KeyPair([]byte(crt), []byte(key)); err != nil {
			err = ErrClientCertificate
			return
		}
	}
	tlsConfig.Certificates = append(tlsConfig.Certificates, certificate)

	return
}
