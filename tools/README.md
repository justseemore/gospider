# 功能概述
* 爬虫小零件库

# 生成根证书
```go
package main
import (
	"log"
	"github.com/justseemore/gospider/tools"
)
func main() {
	key, err := tools.CreateCertKey()
	if err != nil {
		log.Panic(err)
	}
	crt, err := tools.CreateRootCert(key)
	if err != nil {
		log.Panic(err)
	}
	log.Print(crt)
}
```
