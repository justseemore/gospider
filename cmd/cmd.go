package cmd

import (
	"bytes"
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
	"gitee.com/baixudong/gospider/re"
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

// 没有内存泄漏的cmd 客户端
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

// 普通的cmd 客户端
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

var jsScriptVersion = "018"
var pyScriptVersion = "018"

type JyClient struct {
	client *Client
	write  io.WriteCloser
	read   io.ReadCloser
	lock   sync.Mutex
	pip    chan string
}
type PyClientOption struct {
	Script     string   //加载的python 文件
	Names      []string //要调用的函数名称,只有在这里注册的函数名才能被调用
	PythonPath string   //python 的路径,ex: c:/python.exe
	ModulePath []string //python包搜索路径,如果出现搜索不到包的情况,手动在这里加入路径哈
}

// 创建py解析器
func NewPyClient(pre_ctx context.Context, option PyClientOption) (*JyClient, error) {
	if len(option.Names) == 0 {
		return nil, errors.New("缺少调用的函数名,请补充names 字段")
	}
	if option.Script == "" {
		return nil, errors.New("缺少加载的js 文件,请补充script 字段")
	}
	if option.PythonPath == "" {
		option.PythonPath = "python"
	}
	nowDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if option.ModulePath == nil {
		option.ModulePath = []string{nowDir}
	} else {
		option.ModulePath = append(option.ModulePath, nowDir)
	}
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
		Name: option.PythonPath,
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
	pyCli := &JyClient{
		client: cli,
		read:   readBody,
		write:  writeBody,
		pip:    make(chan string),
	}
	go pyCli.readMain()
	jsonData, err := pyCli.run(map[string]any{"Type": "init", "Script": tools.Base64Encode(option.Script), "Names": option.Names, "ModulePath": option.ModulePath})
	if err != nil {
		return nil, err
	}
	errData := jsonData.Get("Error")
	if errData.Exists() && errData.String() != "" {
		return nil, errors.New(errData.String())
	}
	return pyCli, nil
}

type JsClientOption struct {
	Script     string   //加载的js 文件
	Names      []string //要调用的函数名称,只有在这里注册的函数名才能被调用
	NodePath   string   //node 的路径,ex: c:/node.exe
	ModulePath []string //node包搜索路径,如果出现搜索不到包的情况,手动在这里加入路径哈
}

// 创建json解析器
func NewJsClient(pre_ctx context.Context, option JsClientOption) (*JyClient, error) {
	if len(option.Names) == 0 {
		return nil, errors.New("缺少调用的函数名,请补充names 字段")
	}
	if option.Script == "" {
		return nil, errors.New("缺少加载的js 文件,请补充script 字段")
	}
	if option.NodePath == "" {
		option.NodePath = "node"
	}
	nowDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if option.ModulePath == nil {
		option.ModulePath = []string{nowDir}
	} else {
		option.ModulePath = append(option.ModulePath, nowDir)
	}
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
		Name: option.NodePath,
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
	jsCli := &JyClient{
		client: cli,
		read:   readBody,
		write:  writeBody,
		pip:    make(chan string),
	}
	go jsCli.readMain()
	jsonData, err := jsCli.run(map[string]any{"Type": "init", "Script": tools.Base64Encode(option.Script), "Names": option.Names, "ModulePath": option.ModulePath})
	if err != nil {
		return nil, err
	}
	errData := jsonData.Get("Error")
	if errData.Exists() && errData.String() != "" {
		return nil, errors.New(errData.String())
	}
	return jsCli, nil
}
func (obj *JyClient) readMain() {
	defer obj.Close()
	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		allCon := bytes.NewBuffer(nil)
		tempCon := make([]byte, 1024)
		var readInt int
		var err error
		for {
			if readInt, err = obj.read.Read(tempCon); err != nil {
				return
			}
			allCon.Write(tempCon[:readInt])
			if rs := re.Search(`##gospider@start##(.*?)##gospider@end##`, allCon.String()); rs != nil {
				obj.pip <- rs.Group(1)
				allCon.Reset()
			}
		}
	}()
	select {
	case <-obj.client.Done():
		return
	case <-doneChan:
		return
	}
}
func (obj *JyClient) run(dataMap map[string]any) (gjson.Result, error) {
	obj.lock.Lock()
	defer obj.lock.Unlock()
	select {
	case <-obj.client.Done():
		return gjson.Result{}, errors.New("client closed")
	default:
	}
	con, err := json.Marshal(dataMap)
	if err != nil {
		return gjson.Result{}, err
	}
	con = append(con, '\n')
	if _, err = obj.write.Write(con); err != nil {
		return gjson.Result{}, err
	}
	select {
	case data := <-obj.pip:
		return tools.Any2json(data), nil
	case <-obj.client.Done():
		if obj.client.Err != nil {
			return gjson.Result{}, obj.client.Err
		}
		return gjson.Result{}, obj.client.ctx.Err()
	}
}

// 执行函数,第一个参数是要调用的函数名称,后面的是传参
func (obj *JyClient) Call(funcName string, values ...any) (jsonData gjson.Result, err error) {
	if jsonData, err = obj.run(map[string]any{"Type": "call", "Func": funcName, "Args": values}); err != nil {
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
func (obj *JyClient) Close() {
	obj.client.Close()
}

// 运行命令
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

// 导出cmd 的 in管道
func (obj *Client) StdInPipe() (io.WriteCloser, error) {
	return obj.cmd.StdinPipe()
}

// 导出cmd 的 out管道
func (obj *Client) StdOutPipe() (io.ReadCloser, error) {
	return obj.cmd.StdoutPipe()
}

// 导出cmd 的error管道
func (obj *Client) StdErrPipe() (io.ReadCloser, error) {
	return obj.cmd.StderrPipe()
}

// 设置cmd 的 error管道
func (obj *Client) SetStdErr(stderr io.WriteCloser) {
	obj.cmd.Stderr = stderr
}

// 设置cmd 的 out管道
func (obj *Client) SetStdOut(stdout io.WriteCloser) {
	obj.cmd.Stdout = stdout
}

// 设置cmd 的 in管道
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

// 运行是否结束的 chan
func (obj *Client) Done() <-chan struct{} {
	return obj.ctx.Done()
}
