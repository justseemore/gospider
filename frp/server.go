package frp

import (
	"errors"

	"github.com/fatedier/frp/pkg/auth"
	"github.com/fatedier/frp/pkg/config"
	frps "github.com/fatedier/frp/server"
)

type Server struct {
	svr *frps.Service
}

func (obj *Server) Run() {
	obj.svr.Run()
}

type ServerOption struct {
	Host  string //服务端host,默认0.0.0.0
	Port  int    //服务端port
	Token string //密钥，客户端与服务端连接验证
}

func NewServer(option ServerOption) (*Server, error) {
	if option.Token == "" {
		return nil, errors.New("没有token,你想被攻击吗？")
	}
	if option.Host == "" {
		option.Host = "0.0.0.0"
	}
	if option.Port == 0 {
		return nil, errors.New("服务端没有设置监听端口,你确定要这样？")
	}
	svr, err := frps.NewService(
		config.ServerCommonConf{
			MaxPoolCount: 5,
			BindAddr:     option.Host,
			BindPort:     option.Port,
			ServerConfig: auth.ServerConfig{
				BaseConfig: auth.BaseConfig{
					AuthenticationMethod: "token",
				},
				TokenConfig: auth.TokenConfig{
					Token: option.Token,
				},
			},
		},
	)
	return &Server{svr: svr}, err

}
