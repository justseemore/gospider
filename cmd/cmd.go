package cmd

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"gitee.com/baixudong/gospider/conf"
	"gitee.com/baixudong/gospider/tools"
	"github.com/tidwall/gjson"
	"github.com/ysmood/leakless"
	"github.com/ysmood/leakless/pkg/shared"
	"github.com/ysmood/leakless/pkg/utils"
)

type ClientOption struct {
	Name    string   //程序执行的名字
	Args    []string //程序的执行参数
	Dir     string   //程序执行的位置
	TimeOut int      //程序超时时间
}
type Client struct {
	Err error
	cmd *exec.Cmd
	ctx context.Context
	cnl context.CancelFunc
}

func NewLeakClient(preCtx context.Context, option ClientOption) *Client {
	cliCmd := leakless.New()
	name := leakless.GetLeaklessBin()
	leakless.LockPort(cliCmd.Lock)()

	srv, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Print("leak cmd error: ", err)
		return nil
	}
	uid := fmt.Sprintf("%x", utils.RandBytes(16))
	if preCtx == nil {
		preCtx = context.TODO()
	}
	addr := srv.Addr().String()
	option.Args = append([]string{uid, addr, option.Name}, option.Args...)
	option.Name = name
	client := NewClient(preCtx, option)
	go func() {
		defer client.Close()
		defer srv.Close()
		conn, err := srv.Accept()
		if err != nil {
			log.Panic(err)
			return
		}
		enc := json.NewEncoder(conn)
		if err = enc.Encode(shared.Message{UID: uid}); err != nil {
			log.Panic(err)
			return
		}
		dec := json.NewDecoder(conn)
		var msg shared.Message
		if err = dec.Decode(&msg); err != nil {
			log.Panic(err)
			return
		}
		dec.Decode(&msg)
	}()
	return client
}
func NewClient(pre_ctx context.Context, option ClientOption) *Client {
	var cmd *exec.Cmd
	var ctx context.Context
	var cnl context.CancelFunc
	if pre_ctx == nil {
		pre_ctx = context.TODO()
	}
	if option.TimeOut != 0 {
		ctx, cnl = context.WithTimeout(pre_ctx, time.Duration(option.TimeOut)*time.Second)
	} else {
		ctx, cnl = context.WithCancel(pre_ctx)
	}
	cmd = exec.CommandContext(ctx, option.Name, option.Args...)
	cmd.Dir = option.Dir
	result := &Client{
		cmd: cmd,
		ctx: ctx,
		cnl: cnl,
	}
	return result
}

var ErrClosed = errors.New("client closed")

//go:embed cmdPipJsScript.js
var cmdPipJsScript []byte

//go:embed cmdPipPyScript.py
var cmdPipPyScript []byte

var jsScriptVersion = "015"
var pyScriptVersion = "014"

type JsClient struct {
	client *Client
	write  io.WriteCloser
	read   *json.Decoder
	lock   sync.Mutex
}

// 创建py解析器
func NewPyClient(pre_ctx context.Context, script string, name string, names ...string) (*JsClient, error) {
	names = append(names, name)
	userDir, err := conf.GetMainDirPath()
	if err != nil {
		return nil, err
	}
	filePath := tools.PathJoin(userDir, fmt.Sprintf(".cmdPipPyScript%s.py", pyScriptVersion))
	if !tools.PathExist(filePath) {
		err := os.WriteFile(filePath, cmdPipPyScript, 0777)
		if err != nil {
			return nil, err
		}
	}
	cli := NewClient(pre_ctx, ClientOption{
		Name: "python",
		Args: []string{"-u", filePath},
	})
	writeBody, err := cli.StdInPipe()
	if err != nil {
		return nil, err
	}
	readBody, err := cli.StdOutPipe()
	if err != nil {
		return nil, err
	}
	go cli.Run()
	jsCli := &JsClient{
		client: cli,
		read:   json.NewDecoder(readBody),
		write:  writeBody,
	}
	scrJson, err := json.Marshal(map[string]any{"Script": tools.Base64Encode(script), "Names": names})
	if err != nil {
		return nil, err
	}
	jsonData, err := jsCli.run(scrJson)
	if err != nil {
		return nil, err
	}
	errData := jsonData.Get("Error")
	if errData.Exists() && errData.String() != "" {
		return nil, errors.New(errData.String())
	}
	return jsCli, nil
}
func NewJsClient(pre_ctx context.Context, script string, name string, names ...string) (*JsClient, error) {
	names = append(names, name)
	userDir, err := conf.GetMainDirPath()
	if err != nil {
		return nil, err
	}
	filePath := tools.PathJoin(userDir, fmt.Sprintf(".cmdPipJsScript%s.js", jsScriptVersion))
	if !tools.PathExist(filePath) {
		err := os.WriteFile(filePath, cmdPipJsScript, 0777)
		if err != nil {
			return nil, err
		}
	}
	cli := NewClient(pre_ctx, ClientOption{
		Name: "node",
		Args: []string{filePath},
	})
	writeBody, err := cli.StdInPipe()
	if err != nil {
		return nil, err
	}
	readBody, err := cli.StdOutPipe()
	if err != nil {
		return nil, err
	}
	go cli.Run()
	jsCli := &JsClient{
		client: cli,
		read:   json.NewDecoder(readBody),
		write:  writeBody,
	}
	scrJson, err := json.Marshal(map[string]any{"Script": tools.Base64Encode(script), "Names": names})
	if err != nil {
		return nil, err
	}
	jsonData, err := jsCli.run(scrJson)
	if err != nil {
		return nil, err
	}
	errData := jsonData.Get("Error")
	if errData.Exists() && errData.String() != "" {
		return nil, errors.New(errData.String())
	}
	return jsCli, nil
}
func (obj *JsClient) run(con []byte) (gjson.Result, error) {
	obj.lock.Lock()
	defer obj.lock.Unlock()
	con = append(con, '\n')
	_, err := obj.write.Write(con)
	if err != nil {
		return gjson.Result{}, err
	}
	data := map[string]any{}
	err = obj.read.Decode(&data)
	return tools.Any2json(data), err
}

// 执行函数
func (obj *JsClient) Call(funcName string, values ...any) (jsonData gjson.Result, err error) {
	scrJson, _ := json.Marshal(map[string]any{"Func": funcName, "Args": values})
	if jsonData, err = obj.run(scrJson); err != nil {
		if obj.client.Err != nil {
			err = obj.client.Err
		}
		return
	}
	if jsonData.Get("Error").Exists() && jsonData.Get("Error").String() != "" {
		return jsonData.Get("Result"), errors.New(jsonData.Get("Error").String())
	}
	return jsonData.Get("Result"), nil
}

// 关闭解析器
func (obj *JsClient) Close() {
	obj.client.Close()
}

// 运行
func (obj *Client) Run() error {
	defer obj.Close()
	err := obj.cmd.Run()
	if err != nil {
		obj.Err = err
		return obj.Err
	} else if !obj.cmd.ProcessState.Success() {
		if obj.ctx.Err() != nil {
			obj.Err = obj.ctx.Err()
			return obj.Err
		} else {
			obj.Err = errors.New("shell 执行异常")
			return obj.Err
		}
	}
	return obj.Err
}

func (obj *Client) StdInPipe() (io.WriteCloser, error) {
	return obj.cmd.StdinPipe()
}
func (obj *Client) StdOutPipe() (io.ReadCloser, error) {
	return obj.cmd.StdoutPipe()
}
func (obj *Client) StdErrPipe() (io.ReadCloser, error) {
	return obj.cmd.StderrPipe()
}

func (obj *Client) SetStdErr(stderr io.WriteCloser) {
	obj.cmd.Stderr = stderr
}
func (obj *Client) SetStdOut(stdout io.WriteCloser) {
	obj.cmd.Stdout = stdout
}
func (obj *Client) SetStdIn(stdin io.ReadCloser) {
	obj.cmd.Stdin = stdin
}

// 等待运行结束
func (obj *Client) Join() {
	<-obj.ctx.Done()
}

// 关闭客户端
func (obj *Client) Close() {
	obj.cnl()
	if obj.cmd.Process != nil {
		obj.cmd.Process.Kill()
	}
}

func (obj *Client) Done() <-chan struct{} {
	return obj.ctx.Done()
}
