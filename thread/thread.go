package thread

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"sync/atomic"
	"time"

	"gitee.com/baixudong/gospider/chanx"
)

type DefaultClient = Client[bool]

type Client[T any] struct {
	Debug               bool          //是否显示调试信息
	threadStartCallBack func(int64) T //返回请求客户端
	threadEndCallBack   func(T)
	ctx2                context.Context    //控制各个协程
	cnl2                context.CancelFunc //控制各个协程
	ctx                 context.Context    //控制主进程，不会关闭各个协程
	cnl                 context.CancelFunc //控制主进程，不会关闭各个协程
	ctx3                context.Context    //chanx 的协程控制
	cnl3                context.CancelFunc //chanx 的协程控制
	tasks               chan *Task
	sones               chan *Task
	threadTokens        chan struct{}
	tasks2              *chanx.Client[*Task] //chanx 的队列任务
	threadNum           atomic.Int64         //正在运行的协程数量
	timeOut             int
	taskCallBack        func(*Task) error //任务回调
	err                 error
	maxThreadId         atomic.Int64
	threadIds           chan int64
}

type Task struct {
	Func     any                                //运行的函数
	Args     []any                              //传入的参数
	CallBack func(context.Context, []any) error //回调函数
	Timeout  int                                //超时时间
	Result   []any                              //函数执行的结果
	Error    error                              //函数错误信息
	ctx      context.Context
	cnl      context.CancelFunc
}

func (obj *Task) Done() <-chan struct{} {
	return obj.ctx.Done()
}

type BaseClientOption[T any] struct {
	Timeout      int               //任务超时时间
	TaskCallBack func(*Task) error //有序的任务完成回调

	ThreadStartCallBack func(int64) T //每一个线程开始时，根据线程id,创建一个局部对象
	ThreadEndCallBack   func(T)       //线程被消毁时的回调,再这里可以安全的释放局部对象资源
}
type ClientOption = BaseClientOption[bool]

func NewClient(preCtx context.Context, maxNum int, options ...ClientOption) *DefaultClient {
	return NewBaseClient(preCtx, maxNum, options...)
}

func NewBaseClient[T any](preCtx context.Context, maxNum int, options ...BaseClientOption[T]) *Client[T] {
	if preCtx == nil {
		preCtx = context.TODO()
	}
	if maxNum < 1 {
		maxNum = 1
	}
	var option BaseClientOption[T]
	if len(options) > 0 {
		option = options[0]
	}
	if option.Timeout <= 0 {
		option.Timeout = 60
	}
	ctx, cnl := context.WithCancel(preCtx)
	ctx2, cnl2 := context.WithCancel(preCtx)

	tasks := make(chan *Task)
	sones := make(chan *Task)
	threadTokens := make(chan struct{}, maxNum)
	threadIds := make(chan int64, maxNum)
	for i := 0; i < maxNum; i++ {
		threadTokens <- struct{}{}
	}
	pool := &Client[T]{
		threadIds:           threadIds,           //任务id
		taskCallBack:        option.TaskCallBack, //任务回调
		timeOut:             option.Timeout,
		ctx2:                ctx2,
		cnl2:                cnl2, //关闭协程
		ctx:                 ctx,
		cnl:                 cnl,                        //通知关闭
		tasks:               tasks,                      //任务队列
		sones:               sones,                      //write sones
		threadTokens:        threadTokens,               //线程开启总数
		threadStartCallBack: option.ThreadStartCallBack, //线程开始回调
		threadEndCallBack:   option.ThreadEndCallBack,   //线程结束回调
	}
	if option.TaskCallBack != nil { //任务完成回调
		ctx3, cnl3 := context.WithCancel(preCtx)
		pool.tasks2 = chanx.NewClient[*Task](preCtx)
		pool.ctx3 = ctx3
		pool.cnl3 = cnl3
		go pool.taskMain2()
	}
	go pool.taskMain()
	return pool
}
func (obj *Client[T]) getTaskId() int64 { //获取任务id
	select {
	case taskId := <-obj.threadIds:
		return taskId
	default:
		return obj.maxThreadId.Add(1)
	}
}
func (obj *Client[T]) setTaskId(taskId int64) { //回收任务id
	obj.threadIds <- taskId
}

func (obj *Client[T]) caseMain(task *Task) error {
	for {
		select {
		case <-obj.ctx2.Done(): //线程关闭了
			return obj.ctx2.Err()
		case obj.tasks <- task: //加入任务成功
			if obj.tasks2 != nil {
				if err := obj.tasks2.Add(task); err != nil {
					return err
				}
			}
			return obj.Err()
		case <-obj.threadTokens: //等待线程空闲
			go obj.runMain() //开启线程
		}
	}
}

func (obj *Client[T]) taskMain() {
	defer obj.cnl()
	for {
		select {
		case <-obj.ctx2.Done(): //线程关闭推出
			return
		case task := <-obj.sones:
			if err := obj.caseMain(task); err != nil {
				return
			}
		}
	}
}

func (obj *Client[T]) taskMain2() {
	defer obj.cnl3()
	defer obj.Close()
	defer obj.tasks2.Close()
	for task := range obj.tasks2.Chan() {
		select {
		case <-obj.ctx2.Done(): //接到关闭线程通知
			obj.err = obj.ctx2.Err()
		case <-task.Done():
			if task.Error != nil { //任务报错，线程报错
				obj.err = task.Error
			}
		}
		if obj.err != nil { //任务报错，开始关闭线程
			return
		}
		if err := obj.taskCallBack(task); err != nil { //任务回调报错，关闭线程
			obj.err = err
			return
		}
	}
}
func (obj *Client[T]) subThreadNum(runVal T, taskId int64) {
	defer obj.threadNum.Add(-1)       //线程池数量减1
	defer obj.setTaskId(taskId)       //回收线程id
	if obj.threadEndCallBack != nil { //处理回调
		obj.threadEndCallBack(runVal)
	}
	select {
	case <-obj.ctx.Done(): //判断是否是最后一个,如果是最后一个，就关闭线程池
		if obj.Empty() {
			obj.cnl2()
		}
	default:
	}
	obj.threadTokens <- struct{}{} //线程令牌
}
func (obj *Client[T]) runMain() {
	var runVal T
	obj.threadNum.Add(1)                     //正在运行的线程数量加1
	threadId := obj.getTaskId()              //获取线程id
	defer obj.subThreadNum(runVal, threadId) //线程销毁回调
	if obj.threadStartCallBack != nil {      //线程开始回调
		runVal = obj.threadStartCallBack(threadId)
	}
	for {
		select {
		case <-obj.ctx2.Done(): //通知线程关闭
			return
		case <-obj.ctx.Done(): //通知完成任务后关闭
			select {
			case <-obj.ctx2.Done(): //通知线程关闭
				return
			case task := <-obj.tasks: //接收任务
				obj.run(task, runVal, threadId) //运行任务
			default: //没有任务关闭线程
				return
			}
		case task := <-obj.tasks: //接收任务
			obj.run(task, runVal, threadId)
		case <-time.After(time.Second * time.Duration(obj.timeOut)): //等待线程超时
			return
		}
	}
}

var ErrPoolClosed = errors.New("pool closed")

func (obj *Client[T]) verify(fun any, args []any) error {
	if fun == nil {
		return errors.New("not func")
	}
	typeOfFun := reflect.TypeOf(fun)
	index := 1
	if obj.threadStartCallBack != nil {
		index = 2
		var tmpVal T
		if reflect.TypeOf(tmpVal).String() != typeOfFun.In(1).String() {
			return fmt.Errorf("two params not %T", tmpVal)
		}
	}
	if typeOfFun.Kind() != reflect.Func {
		return errors.New("not func")
	}
	if typeOfFun.NumIn() != len(args)+index {
		return errors.New("args num error")
	}
	if typeOfFun.In(0).String() != "context.Context" {
		return errors.New("frist params not context.Context")
	}
	for i := index; i < len(args)+index; i++ {
		if args[i-index] == nil {
			if reflect.Zero(typeOfFun.In(i)).Interface() != args[i-index] {
				return errors.New("args type not equel")
			}
		} else if !reflect.TypeOf(args[i-index]).ConvertibleTo(typeOfFun.In(i)) {
			return errors.New("args type not equel")
		}
	}
	return nil
}

// 创建task
func (obj *Client[T]) Write(task *Task) (*Task, error) {
	task.ctx, task.cnl = context.WithCancel(obj.ctx2)        //设置任务ctx
	if err := obj.verify(task.Func, task.Args); err != nil { //验证参数
		task.Error = err
		task.cnl()
		return task, err
	}
	select {
	case <-obj.ctx2.Done(): //接到线程关闭通知
		if obj.Err() != nil {
			task.Error = obj.Err()
		} else {
			task.Error = ErrPoolClosed
		}
		task.cnl()
		return task, task.Error
	case <-obj.ctx.Done(): //接到线程关闭通知
		if obj.Err() != nil {
			task.Error = obj.Err()
		} else {
			task.Error = ErrPoolClosed
		}
		task.cnl()
		return task, task.Error
	case obj.sones <- task: //将任务传递到sones
		return task, nil
	}
}

type myInt int64

var ThreadId myInt = 0

func GetThreadId(ctx context.Context) int64 { //获取线程id，获取失败返回0
	if ctx == nil {
		return 0
	}
	if val := ctx.Value(ThreadId); val != nil {
		if v, ok := val.(int64); ok {
			return v
		}
	}
	return 0
}
func (obj *Client[T]) run(task *Task, option T, threadId int64) {
	defer func() {
		if r := recover(); r != nil {
			task.Error = fmt.Errorf("%v", r)
			if obj.Debug {
				debug.PrintStack()
			}
		}
	}()
	timeOut := task.Timeout
	if timeOut > 0 {
		task.ctx, task.cnl = context.WithTimeout(task.ctx, time.Second*time.Duration(timeOut))
	}
	defer task.cnl()                                       //函数结束，任务完成
	ctx := context.WithValue(task.ctx, ThreadId, threadId) //线程id 值写入ctx
	index := 1
	if obj.threadStartCallBack != nil {
		index = 2
	}
	params := make([]reflect.Value, len(task.Args)+index)
	params[0] = reflect.ValueOf(ctx)
	if obj.threadStartCallBack != nil {
		params[1] = reflect.ValueOf(option)
	}
	for k, param := range task.Args {
		params[k+index] = reflect.ValueOf(param)
	}
	task.Result = []any{}
	for _, rs := range reflect.ValueOf(task.Func).Call(params) { //执行主方法
		task.Result = append(task.Result, rs.Interface())
	}
	if task.CallBack != nil {
		task.Error = task.CallBack(ctx, task.Result) //执行回调方法
	}
}

func (obj *Client[T]) Join() error { //等待所有任务完成，并关闭pool
	obj.cnl()
	if obj.tasks2 != nil {
		obj.tasks2.Join()
		<-obj.ctx3.Done()
	}
loop:
	for {
		select {
		case <-obj.ctx2.Done(): //等待所有的任务执行完毕
			break loop
		case <-time.After(time.Second):
			if obj.Empty() {
				obj.cnl2()
			}
		}
	}
	return obj.Err()
}

func (obj *Client[T]) Close() { //告诉所有协程，立即结束任务
	obj.cnl()
	obj.cnl2()
	if obj.tasks2 != nil {
		obj.tasks2.Close()
	}
}
func (obj *Client[T]) Err() error { //错误
	return obj.err
}
func (obj *Client[T]) Done() <-chan struct{} { //所有任务执行完毕
	return obj.ctx2.Done()
}
func (obj *Client[T]) ThreadSize() int64 { //创建的协程数量
	return obj.threadNum.Load()
}
func (obj *Client[T]) Empty() bool { //任务是否为空
	if obj.ThreadSize() <= 0 && len(obj.sones) == 0 && len(obj.tasks) == 0 {
		return true
	}
	return false
}
