# Function Overview
- Lightning-fast thread pool
## Multi-threading Example
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
    log.Print("Finished")
}
```
## Get Thread ID
```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/justseemore/gospider/thread"
)

func test(ctx context.Context, num int) {
    log.Printf("Thread %d in thread pool %d starts", thread.GetThreadId(ctx), thread.GetThreadPoolId(ctx))
    time.Sleep(time.Second)
    log.Printf("Thread %d in thread pool %d ends", thread.GetThreadId(ctx), thread.GetThreadPoolId(ctx))
}

func main() {
    threadCli := thread.NewClient(nil, 3) // Limit concurrency to 3
    for i := 0; i < 10; i++ {
        // Write tasks
        threadCli.Write(&thread.Task{
            Func: test,
            Args: []any{i},
        })
    }
    threadCli.Join()
}
```
