# Function Overview
* Execute command-line commands
* Execute cmd commands without memory leaks
* JavaScript interpreter, used with Node.js, execute JavaScript code using pipes
* Python interpreter, used with Python, execute Python code using pipes

## Execute JavaScript Code Example
```go
package main

import (
	"log"

	"github.com/justseemore/gospider/cmd"
)

func TestJs() {
	script := `
	function sign(val,val2){
		return {"signval":val,"signval2":val2}
	}
    function sign2(val,val2){
		return {"sign2val":val,"sign2val2":val2}
	}
	`
	jsCli, err := cmd.NewJsClient(nil, cmd.JsClientOption{
		Script: script,
		Names:  []string{"sign", "sign2"},
	})
	if err != nil {
		log.Fatal(err)
	}
	rs, err := jsCli.Call("sign", 1, 2)
	if err != nil {
		log.Fatal(err)
	}
	if rs.Get("signval").Int() != 1 || rs.Get("signval2").Int() != 2 {
		log.Fatal("sign error")
	}
	rs, err = jsCli.Call("sign2", 1, 2)
	if err != nil {
		log.Fatal(err)
	}
	if rs.Get("sign2val").Int() != 1 || rs.Get("sign2val2").Int() != 2 {
		log.Fatal("sign error")
	}
}
```
## Execute Python Code Example
```go
func TestPy() {
	script := `def sign(val,val2):
	return {"val":val,"val2":val2}`
	pyCli, err := cmd.NewPyClient(nil, cmd.PyClientOption{
		Script: script,
		Names:  []string{"sign"},
	})
	if err != nil {
		log.Fatal(err)
	}
	rs, err := pyCli.Call("sign", 1, 2)
	if err != nil {
		log.Fatal(err)
	}
	if rs.Get("val").Int() != 1 || rs.Get("val2").Int() != 2 {
		log.Fatal("sign error")
	}
}
```
