package linux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gitee.com/baixudong/gospider/re"
	"gitee.com/baixudong/gospider/tools"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type ClientOption struct {
	Timeout int    //连接超时时间
	Host    string //远程host
	Port    int    //远程port
	Usr     string //用户名
	Pwd     string //密码
	KeyPath string //key文件的路径,当使用key登陆时使用
	KeyData []byte //key文件的内容,当使用key登陆时使用
}
type Client struct {
	client *ssh.Client
	pwd    string
}

type Ssh struct {
	client *ssh.Session
}
type Sftp struct {
	client *sftp.Client
}
type Screen struct {
	IsRun    bool
	client   *Ssh
	pip      *ScreenPip
	waitTime int64
	pwd      string
	ctx      context.Context
	cnl      context.CancelFunc
}
type ScreenPip struct {
	inData   chan []byte
	outData  chan []byte
	waitTime int64
	ctx      context.Context
	cnl      context.CancelFunc
}

func (obj *ScreenPip) Read(con []byte) (int, error) {
	select {
	case ron := <-obj.inData:
		return copy(con, ron), nil
	case <-obj.ctx.Done():
		return 0, io.EOF
	}
}
func (obj *ScreenPip) Write(con []byte) (int, error) {
	select {
	case obj.outData <- con:
		return len(con), nil
	case <-obj.ctx.Done():
		return 0, io.EOF
	case <-time.After(time.Second * time.Duration(obj.waitTime)):
		return 0, io.EOF
	}
}
func (obj *ScreenPip) Close() error {
	obj.cnl()
	return nil
}

type TermOption struct {
	Type     string //defalut  xterm
	DisEcho  bool   // 是否禁用回显
	InSpeed  uint32 // input speed = 14.4kbaud
	OutSpeed uint32 //output speed = 14.4kbaud
	Row      int
	Col      int
}

func NewClient(option ClientOption) (*Client, error) {
	config := &ssh.ClientConfig{
		User:            option.Usr,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if option.Timeout == 0 {
		option.Timeout = 30
	}
	config.Timeout = time.Second * time.Duration(option.Timeout)
	if option.Pwd != "" {
		config.Auth = []ssh.AuthMethod{ssh.Password(option.Pwd)}
	} else if option.KeyData != nil {
		signer, err := ssh.ParsePrivateKey(option.KeyData)
		if err != nil {
			return nil, err
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	} else if option.KeyPath != "" {
		if tools.PathExist(option.KeyPath) {
			key, err := os.ReadFile(option.KeyPath)
			if err != nil {
				return nil, err
			}
			signer, err := ssh.ParsePrivateKey(key)
			if err != nil {
				return nil, err
			}
			config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
		} else {
			return nil, errors.New("key path is not found")
		}
	} else {
		return nil, errors.New("请输入密码")
	}
	addr := fmt.Sprintf("%s:%d", option.Host, option.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}
	return &Client{client: client, pwd: option.Pwd}, nil
}
func (obj *Client) Close() error {
	return obj.client.Close()
}
func (obj *Client) NewSftp() (*Sftp, error) {
	session, err := sftp.NewClient(obj.client)
	if err != nil {
		return nil, err
	}
	return &Sftp{client: session}, err
}
func (obj *Client) NewSsh() (*Ssh, error) {
	client, err := obj.client.NewSession()
	if err != nil {
		return nil, err
	}
	return &Ssh{client: client}, err
}
func (obj *Client) NewScreen(preCtx context.Context, name string, waitTimes ...int64) (*Screen, error) {
	if preCtx == nil {
		preCtx = context.TODO()
	}
	client, err := obj.NewSsh()
	if err != nil {
		return nil, err
	}
	var waitTime int64
	if len(waitTimes) > 0 {
		waitTime = waitTimes[0]
	} else {
		waitTime = 5
	}
	ctx, cnl := context.WithCancel(preCtx)
	pip := ScreenPip{inData: make(chan []byte), outData: make(chan []byte), waitTime: waitTime, ctx: ctx, cnl: cnl}
	client.SetStdIn(&pip)
	client.SetStdOut(&pip)
	client.SetStdErr(&pip)
	if err := client.Term(); err != nil {
		cnl()
		client.Close()
		return nil, err
	}
	screen := &Screen{
		client:   client,
		pip:      &pip,
		waitTime: waitTime,
		pwd:      obj.pwd,
		ctx:      ctx,
		cnl:      cnl,
	}
	if !screen.reg(tools.BytesToString(screen.bytes())) {
		return nil, errors.New("打开 linux 失败")
	}
	cmd := []byte(fmt.Sprintf("screen -x %s\n", name))
	runCon, err := screen.Run(cmd)
	if err != nil {
		screen.Close()
		return nil, err
	}
	screenCon := tools.BytesToString(runCon)
	if strings.Contains(screenCon, "here is no screen to be attached matching") {
		screen.Close()
		return nil, errors.New("screen不存在")
	}
	return screen, nil
}
func (obj *Screen) reg(txt string) bool { //是否匹配到命令行前缀
	res := re.Search(`[$#](.*)$`, txt)
	if res == nil {
		return false
	}
	if res.Group(1)[0] == 32 {
		return true
	}
	return false
}
func (obj *Screen) bytes() []byte {
	var allCon []byte
	lastTime := time.Now().Unix() + obj.waitTime
	for {
		select {
		case con := <-obj.pip.outData:
			allCon = append(allCon, con...)
			if time.Now().Unix() > lastTime {
				obj.IsRun = true
				return allCon
			}
		case <-time.After(time.Second * time.Duration(obj.waitTime)):
			lastLine := re.Search(`\n[^\n]*?$`, tools.BytesToString(allCon))

			if lastLine != nil && !obj.reg(lastLine.Group()) {
				obj.IsRun = true
			} else {
				obj.IsRun = false
			}
			return allCon
		}
	}
}
func (obj *Screen) Run(cmd []byte) ([]byte, error) {
	select {
	case obj.pip.inData <- cmd:
		return obj.bytes(), nil
	case <-time.After(time.Second * time.Duration(obj.waitTime)):
		return nil, errors.New("timeOut")
	}
}
func (obj *Screen) SudoRun(cmd []byte) ([]byte, error) {
	allCon, err := obj.Run(cmd)
	if err != nil {
		return allCon, err
	}
	if obj.pwd != "" {
		rs := re.Search(`\n\[sudo\].*?[：:]\s*?$`, string(allCon))
		if rs != nil {
			runCon, err := obj.Run([]byte(fmt.Sprintf("%s\n", obj.pwd)))
			if err != nil {
				return allCon, err
			}
			allCon = append(allCon, runCon...)
		}
	}
	return allCon, nil
}
func (obj *Screen) Close() error {
	obj.cnl()
	return obj.client.Close()
}
func (obj *Ssh) Run(cmd string) ([]byte, error) {
	return obj.client.Output(cmd)
}
func (obj *Ssh) Term(options ...TermOption) error {
	var option TermOption
	if len(options) > 0 {
		option = options[0]
	}
	if option.InSpeed == 0 {
		option.InSpeed = 14400
	}
	if option.OutSpeed == 0 {
		option.OutSpeed = 14400
	}
	if option.Type == "" {
		option.Type = "xterm-256color"
	}
	var echo uint32
	if !option.DisEcho {
		echo = 1
	}
	if obj.client.Stdin == nil {
		obj.SetStdIn(os.Stdin)
	}
	if obj.client.Stdout == nil {
		obj.SetStdOut(os.Stdout)
	}
	if obj.client.Stderr == nil {
		obj.SetStdErr(os.Stderr)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          echo,            // 禁用回显（0禁用，1启动）
		ssh.TTY_OP_ISPEED: option.InSpeed,  // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: option.OutSpeed, //output speed = 14.4kbaud
	}
	if err := obj.client.RequestPty(option.Type, option.Row, option.Col, modes); err != nil {
		return err
	}
	return obj.client.Shell()
}
func (obj *Ssh) SetStdIn(val io.Reader) {
	obj.client.Stdin = val
}
func (obj *Ssh) SetStdOut(val io.Writer) {
	obj.client.Stdout = val
}
func (obj *Ssh) SetStdErr(val io.Writer) {
	obj.client.Stderr = val
}

func (obj *Ssh) StdErrPipe() (io.Reader, error) {
	return obj.client.StderrPipe()
}
func (obj *Ssh) StdInPipe() (io.WriteCloser, error) {
	return obj.client.StdinPipe()
}
func (obj *Ssh) StdOutPipe() (io.Reader, error) {
	return obj.client.StdoutPipe()
}

func (obj *Ssh) Close() error {
	return obj.client.Close()
}

func (obj *Sftp) Upload(local_path string, remote_paths ...string) error {
	if !tools.PathExist(local_path) {
		return errors.New("local path not found")
	}

	local_path_info, err := os.Stat(local_path)
	if err != nil {
		return err
	}
	var remote_path string
	if len(remote_paths) == 0 {
		remote_path, err = obj.client.Getwd()
		if err != nil {
			return err
		}
	} else {
		remote_path = remote_paths[0]
		remote_path_info, err := obj.client.Stat(remote_path)
		if err != nil {
			return err
		}
		if !remote_path_info.IsDir() {
			return errors.New("remote  not is dir")
		}
	}
	if local_path_info.IsDir() {
		return obj.UploadDir(local_path, obj.client.Join(remote_path, local_path_info.Name()))
	}
	return obj.UploadFile(local_path, obj.client.Join(remote_path, local_path_info.Name()))
}
func (obj *Sftp) UploadFile(local_path string, remote_path string) error {
	dstFile, err := obj.client.Create(remote_path)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	local_content, err := os.ReadFile(local_path)
	if err != nil {
		return err
	}
	dstFile.Write(local_content)
	return nil
}
func (obj *Sftp) UploadDir(local_path string, remote_path string) error {
	err := obj.client.Mkdir(remote_path)
	if err != nil {
		return err
	}
	localFiles, err := os.ReadDir(local_path)
	if err != nil {
		return err
	}
	for _, localFile := range localFiles {
		if localFile.IsDir() {
			err := obj.UploadDir(path.Join(local_path, localFile.Name()), obj.client.Join(remote_path, localFile.Name()))
			if err != nil {
				return err
			}
		} else {
			err := obj.UploadFile(path.Join(local_path, localFile.Name()), obj.client.Join(remote_path, localFile.Name()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func (obj *Sftp) Download(remote_path string, local_paths ...string) error {
	remote_path_info, err := obj.client.Stat(remote_path)
	if err != nil {
		return err
	}
	var local_path string
	if len(local_paths) == 0 {
		_, dir, _, ok := runtime.Caller(1)
		if !ok {
			return errors.New("获取当前目录错误")
		}
		local_path = filepath.Dir(dir)
	} else {
		local_path = local_paths[0]
		local_path_info, err := os.Stat(local_path)
		if err != nil {
			return err
		}
		if !local_path_info.IsDir() {
			return errors.New("local path not is dir")
		}
	}
	if remote_path_info.IsDir() {
		return obj.DownloadDir(remote_path, path.Join(local_path, remote_path_info.Name()))
	}
	return obj.DownloadFile(remote_path, path.Join(local_path, remote_path_info.Name()))
}
func (obj *Sftp) DownloadFile(remote_path string, local_path string) error {
	remote_file, err := obj.client.Open(remote_path)
	if err != nil {
		return err
	}
	defer remote_file.Close()
	dstFile, err := os.Create(local_path)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = remote_file.WriteTo(dstFile)
	return err
}
func (obj *Sftp) DownloadDir(remote_path string, local_path string) error {
	err := tools.MkDir(local_path)
	if err != nil {
		return err
	}
	remotefiles, err := obj.client.ReadDir(remote_path)
	if err != nil {
		return err
	}
	for _, remotefile := range remotefiles {
		if remotefile.IsDir() {
			err := obj.DownloadDir(obj.client.Join(remote_path, remotefile.Name()), path.Join(local_path, remotefile.Name()))
			if err != nil {
				return err
			}
		} else {
			err := obj.DownloadFile(obj.client.Join(remote_path, remotefile.Name()), path.Join(local_path, remotefile.Name()))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func (obj *Sftp) Close() error {
	return obj.client.Close()
}
