# 功能概要
* 执行命令行命令
* 没有内存泄露的执行cmd 命令
* js 解析器，配合node ,调用js代码，使用管道
* python 解析器，配合python ,调用python代码，使用管道

## 执行js 代码示例
~~~go
package main

import (
	"log"

	"gitee.com/baixudong/gospider/cmd"
)
func main() {
	script := `
	function sign(val,val2){
		return {"signval":val,"signval2":val2}
	}
    function sign2(val,val2){
		return {"sign2val":val,"sign2val2":val2}
	}
	`
	jsCli, err := cmd.NewJsClient(nil, script, "sign", "sign2")
	if err != nil {
		log.Panic(err)
	}
	rs, err := jsCli.Call("sign", 1, 2)
	if err != nil {
		log.Panic(err)
	}
	log.Print(rs)
	rs, err = jsCli.Call("sign2", 1, 2)
	if err != nil {
		log.Panic(err)
	}
	log.Print(rs)
}
~~~