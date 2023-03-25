# 功能概述
* 快如闪电的线程池
## 多线程示例
```go
func main() {
	pool := thread.NewClient(nil, 3)
	for i := 0; i < 20; i++ {
		_, err := pool.Write(&thread.Task{
			Func: func(ctx context.Context, i int) {
				log.Print(i, "start")
				time.Sleep(time.Second)
				log.Print(i, "end")
			},
			Args: []any{i},
		})
		if err != nil {
			log.Panic(err)
		}
	}
	pool.Join()
	log.Print("结束了")
}
```

