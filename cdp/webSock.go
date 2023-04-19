package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/db"
	"gitee.com/baixudong/gospider/ja3"
	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/thread"
	"gitee.com/baixudong/gospider/websocket"

	"go.uber.org/atomic"
)

type commend struct {
	Id     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}
type event struct {
	Ctx      context.Context
	Cnl      context.CancelFunc
	RecvData chan RecvData
}
type RecvError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type RecvData struct {
	Id     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
	Result map[string]any `json:"result"`
	Error  RecvError      `json:"error"`
}

type WebSock struct {
	option       WebSockOption
	db           *db.Client[FulData]
	ids          map[int64]*event
	methodLock   sync.RWMutex
	idLock       sync.RWMutex
	conn         *websocket.Conn
	ctx          context.Context
	cnl          context.CancelCauseFunc
	id           atomic.Int64
	RequestFunc  func(context.Context, *Route)
	ResponseFunc func(context.Context, *Route)
	reqCli       *requests.Client
	onEvents     map[string]func(ctx context.Context, rd RecvData)
}

type DataEntrie struct {
	Bytes string `json:"bytes"`
}

func (obj *WebSock) Done() <-chan struct{} {
	return obj.ctx.Done()
}
func (obj *WebSock) routeMain(ctx context.Context, recvData RecvData) {
	routeData := RouteData{}
	temData, err := json.Marshal(recvData.Params)
	if err == nil && json.Unmarshal(temData, &routeData) == nil {
		route := &Route{
			webSock:  obj,
			recvData: routeData,
		}
		if route.IsResponse() {
			if obj.ResponseFunc != nil {
				obj.ResponseFunc(ctx, route)
			} else {
				route.Continue(ctx)
			}
		} else {
			if obj.RequestFunc != nil {
				obj.RequestFunc(ctx, route)
			} else {
				route.Continue(ctx)
			}
		}
	}
}

func (obj *WebSock) recv(ctx context.Context, rd RecvData) error {
	obj.idLock.RLock()
	cmdData, ok := obj.ids[rd.Id]
	obj.idLock.RUnlock()
	if ok {
		select {
		case <-obj.Done():
			return errors.New("websocks closed")
		case <-ctx.Done():
			return ctx.Err()
		case <-cmdData.Ctx.Done():
			obj.idLock.Lock()
			delete(obj.ids, rd.Id)
			obj.idLock.Unlock()
		case cmdData.RecvData <- rd:
		}
	}
	obj.methodLock.RLock()
	methodFunc, ok := obj.onEvents[rd.Method]
	obj.methodLock.RUnlock()
	if ok && methodFunc != nil {
		methodFunc(ctx, rd)
	}
	return nil
}
func (obj *WebSock) recvMain() (err error) {
	defer obj.Close(err)
	pool := thread.NewClient(obj.ctx, 65535)
	defer pool.Close()
	for {
		select {
		case <-obj.ctx.Done():
			return obj.ctx.Err()
		default:
			rd := RecvData{}
			if err := obj.conn.RecvJson(obj.ctx, &rd); err != nil {
				return err
			}
			if rd.Id == 0 {
				rd.Id = obj.id.Add(1)
			}
			if _, err := pool.Write(&thread.Task{
				Func: obj.recv,
				Args: []any{rd},
			}); err != nil {
				return err
			}
		}
	}
}

type WebSockOption struct {
	Proxy     string
	DataCache bool //开启数据缓存
	Ja3Spec   ja3.ClientHelloSpec
	Ja3       bool
}

func NewWebSock(preCtx context.Context, globalReqCli *requests.Client, ws string, option WebSockOption, db *db.Client[FulData]) (*WebSock, error) {
	response, err := globalReqCli.Request(preCtx, "get", ws, requests.RequestOption{DisProxy: true})
	if err != nil {
		return nil, err
	}
	response.WebSocket().SetReadLimit(1024 * 1024 * 1024) //1G
	cli := &WebSock{
		ids:      make(map[int64]*event),
		conn:     response.WebSocket(),
		db:       db,
		reqCli:   globalReqCli,
		option:   option,
		onEvents: map[string]func(ctx context.Context, rd RecvData){},
	}
	cli.ctx, cli.cnl = context.WithCancelCause(preCtx)
	go cli.recvMain()
	cli.AddEvent("Fetch.requestPaused", cli.routeMain)
	return cli, err
}
func (obj *WebSock) AddEvent(method string, fun func(ctx context.Context, rd RecvData)) {
	obj.methodLock.Lock()
	obj.onEvents[method] = fun
	obj.methodLock.Unlock()
}
func (obj *WebSock) DelEvent(method string) {
	obj.methodLock.Lock()
	delete(obj.onEvents, method)
	obj.methodLock.Unlock()
}

func (obj *WebSock) Close(err error) error {
	obj.cnl(err)
	return obj.conn.Close("close")
}

func (obj *WebSock) regId(preCtx context.Context, ids ...int64) *event {
	data := new(event)
	data.Ctx, data.Cnl = context.WithCancel(preCtx)
	data.RecvData = make(chan RecvData)
	for _, id := range ids {
		obj.idLock.Lock()
		obj.ids[id] = data
		obj.idLock.Unlock()
	}
	return data
}
func (obj *WebSock) send(preCtx context.Context, cmd commend) (RecvData, error) {
	var cnl context.CancelFunc
	var ctx context.Context
	if preCtx == nil {
		ctx, cnl = context.WithTimeout(obj.ctx, time.Second*60)
	} else {
		ctx, cnl = context.WithTimeout(preCtx, time.Second*60)
	}
	defer cnl()
	select {
	case <-obj.Done():
		return RecvData{}, context.Cause(obj.ctx)
	case <-ctx.Done():
		return RecvData{}, obj.ctx.Err()
	default:
		cmd.Id = obj.id.Add(1)
		idEvent := obj.regId(ctx, cmd.Id)
		defer idEvent.Cnl()
		if err := obj.conn.SendJson(ctx, cmd); err != nil {
			return RecvData{}, err
		}
		select {
		case <-obj.Done():
			return RecvData{}, context.Cause(obj.ctx)
		case <-ctx.Done():
			return RecvData{}, ctx.Err()
		case idRecvData := <-idEvent.RecvData:
			if idRecvData.Error.Message != "" {
				return idRecvData, errors.New(idRecvData.Error.Message)
			}
			return idRecvData, nil
		}
	}
}
