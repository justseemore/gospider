# Function Overview
- Cookie switch, connection pool switch, HTTP/2, JA3
- Self-implemented SOCKS5, HTTP proxy, HTTPS proxy
- Automatic decompression and decoding
- DNS caching
- Automatic type conversion
- Retry attempts, request callbacks
- WebSocket protocol
- SSE protocol
# Setting Proxies
## Proxy Setting Priority
```
Global proxy method < Global proxy string < Local proxy string
```
## Set and Modify Global Proxy Method (Only called when creating a new connection, not called when reusing connections)
```golang
package main

import (
    "log"

    "gitee.com/baixudong/gospider/requests"
)

func main() {
	reqCli, err := requests.NewClient(nil, requests.ClientOption{
		GetProxy: func(ctx context.Context, url *url.URL) (string, error) { // Set global proxy method
			return "http://127.0.0.1:7005", nil
		}})
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "http://myip.top") // Send GET request
	if err != nil {
		log.Panic(err)
	}
	reqCli.SetGetProxy(func(ctx context.Context, url *url.URL) (string, error) { // Modify global proxy method
		return "http://127.0.0.1:7006", nil
	})
	log.Print(response.Text()) // Get content and parse as string
}
```
## Set and Modify Global Proxy
```golang
package main

import (
    "log"

    "gitee.com/baixudong/gospider/requests"
)

func main() {
	reqCli, err := requests.NewClient(nil, requests.ClientOption{
		Proxy: "http://127.0.0.1:7005", // Set global proxy
	})
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "http://myip.top") // Send GET request
	if err != nil {
		log.Panic(err)
	}
	err = reqCli.SetProxy("http://127.0.0.1:7006") // Modify global proxy
	if err != nil {
		log.Panic(err)
	}
	log.Print(response.Text()) // Get content and parse as string
}
```
## Set Local Proxy
```golang
package main

import (
    "log"

    "gitee.com/baixudong/gospider/requests"
)

func main() {
	reqCli, err := requests.NewClient(nil)
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "http://myip.top", requests.RequestOption{
		Proxy: "http://127.0.0.1:7005",
	})
	if err != nil {
		log.Panic(err)
	}
	log.Print(response.Text()) // Get content and parse as string
}
```
## Force Close Proxy and Use Local Network
```golang
package main

import (
    "log"

    "gitee.com/baixudong/gospider/requests"
)

func main() {
	reqCli, err := requests.NewClient(nil)
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "http://myip.top", requests.RequestOption{
		DisProxy: true, // Force using local proxy
	})
	if err != nil {
		log.Panic(err)
	}
	log.Print(response.Text()) // Get content and parse as string
}
```

# Sending HTTP Requests

```golang
package main

import (
    "log"

    "gitee.com/baixudong/gospider/requests"
)

func main() {
    reqCli, err := requests.NewClient(nil) // Create request client
    if err != nil {
        log.Panic(err)
    }
    response, err := reqCli.Request(nil, "get", "http://myip.top") // Send GET request
    if err != nil {
        log.Panic(err)
    }
    log.Print(response.Text())    // Get content and parse as string
    log.Print(response.Content()) // Get content as bytes
    log.Print(response.Json())    // Get JSON and parse with gjson
    log.Print(response.Html())    // Get content and parse as DOM
    log.Print(response.Cookies()) // Get cookies
}

```

# Sending WebSocket Requests

```golang
package main

import (
	"context"
	"log"

	"gitee.com/baixudong/gospider/requests"
	"gitee.com/baixudong/gospider/websocket"
)

func main() {
	reqCli, err := requests.NewClient(nil) // Create request client
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "ws://82.157.123.54:9010/ajaxchattest", requests.RequestOption{Headers: map[string]string{
		"Origin": "http://coolaf.com",
	}}) // Send WebSocket request
	if err != nil {
		log.Panic(err)
	}
	defer response.Close()
	wsCli := response.WebSocket()
	wsCli.SetReadLimit(1024 * 1024 * 1024) // Set maximum read limit
	if err = wsCli.Send(context.TODO(), websocket.MessageText, "测试"); err != nil { // Send text message
		log.Panic(err)
	}
	msgType, con, err := wsCli.Recv(context.TODO()) // Receive message
	if err != nil {
		log.Panic(err)
	}
	log.Print(msgType)     // Message type
	log.Print(string(con)) // Message content
}
```
# IPv4, IPv6 Address Control Parsing
```go
func main() {
	reqCli, err := requests.NewClient(nil, requests.ClientOption{
		AddrType: requests.Ipv4, // Prioritize parsing IPv4 addresses
		// AddrType: requests.Ipv6, // Prioritize parsing IPv6 addresses
	})
	if err != nil {
		log.Panic(err)
	}
	href := "https://test.ipw.cn"
	resp, err := reqCli.Request(nil, "get", href)
	if err != nil {
		log.Panic(err)
	}
	log.Print(resp.Text())
	log.Print(resp.StatusCode())
}
``` 
# Forge JA3 Fingerprints
## Generate Fingerprint from String
```go
func main() {
	ja3Str := "772,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-17513,29-23-24,0"
	Ja3Spec, err := ja3.CreateSpecWithStr(ja3Str) // Generate fingerprint from string
	if err != nil {
		log.Panic(err)
	}
	reqCli, err := requests.NewClient(nil, requests.ClientOption{Ja3Spec: Ja3Spec})
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "https://tools.scrapfly.io/api/fp/ja3?extended=1")
	if err != nil {
		log.Panic(err)
	}
	jsonData,_:=response.Json()
	log.Print(jsonData.Get("ja3").String())
	log.Print(jsonData.Get("ja3").String() == ja3Str)
}
```
## Generate Fingerprint from ID
```go
func main() {
	Ja3Spec, err := ja3.CreateSpecWithId(ja3.HelloChrome_Auto) // Generate fingerprint from ID
	if err != nil {
		log.Panic(err)
	}
	reqCli, err := requests.NewClient(nil, requests.ClientOption{Ja3Spec: Ja3Spec})
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "https://tools.scrapfly.io/api/fp/ja3?extended=1")
	if err != nil {
		log.Panic(err)
	}
	jsonData,_:=response.Json()
	log.Print(jsonData.Get("ja3").String())
}
```
## JA3 Switch
```go
func main() {
	reqCli, err := requests.NewClient(nil)
	if err != nil {
		log.Panic(err)
	}
	response, err := reqCli.Request(nil, "get", "https://tools.scrapfly.io/api/fp/ja3?extended=1", requests.RequestOption{Ja3: true}) // Use the latest Chrome fingerprint
	if err != nil {
		log.Panic(err)
	}
	jsonData,_:=response.Json()
	log.Print(jsonData.Get("ja3").String())
}
```
## H2 Fingerprint Switch
```go
func main() {
	reqCli, err := requests.NewClient(nil, requests.ClientOption{
		H2Ja3: true,
	})
	if err != nil {
		log.Panic(err)
	}
	href := "https://tools.scrapfly.io/api/fp/anything"
	resp, err := reqCli.Request(nil, "get", href)
	if err != nil {
		log.Panic(err)
	}
	log.Print(resp.Text())
}
```
## Modify H2 Fingerprint
```go
func main() {
	reqCli, err := requests.NewClient(nil, requests.ClientOption{
		H2Ja3Spec: ja3.H2Ja3Spec{
			InitialSetting: []ja3.Setting{
				{Id: 1, Val: 65555},
				{Id: 2, Val: 1},
				{Id: 3, Val: 2000},
				{Id: 4, Val: 6291457},
				{Id: 6, Val: 262145},
			},
			ConnFlow: 15663106,
			OrderHeaders: []string{
				":method",
				":path",
				":scheme",
				":authority",
			},
		},
	})
	if err != nil {
		log.Panic(err)
	}
	href := "https://tools.scrapfly.io/api/fp/anything"
	resp, err := reqCli.Request(nil, "get", href)
	if err != nil {
		log.Panic(err)
	}
	log.Print(resp.Text())
}
```
# Collecting Title of List Pages from National Public Resource Website and China Government Procurement Website
```go
package main
import (
	"log"
	"gitee.com/baixudong/gospider/requests"
)
func main() {
	reqCli, err := requests.NewClient(nil)
	if err != nil {
		log.Panic(err)
	}
	resp, err := reqCli.Request(nil, "get", "http://www.ccgp.gov.cn/cggg/zygg/")
	if err != nil {
		log.Panic(err)
	}
	html := resp.Html()
	lis := html.Finds("ul.c_list_bid li")
	for _, li := range lis {
		title := li.Find("a").Get("title")
		log.Print(title)
	}
	resp, err = reqCli.Request(nil, "post", "http://deal.ggzy.gov.cn/ds/deal/dealList_find.jsp", requests.RequestOption{
		Data: map[string]string{
			"TIMEBEGIN_SHOW": "2023-04-26",
			"TIMEEND_SHOW":   "2023-05-05",
			"TIMEBEGIN":      "2023-04-26",
			"TIMEEND":        "2023-05-05",
			"SOURCE_TYPE":    "1",
			"DEAL_TIME":      "02",
			"DEAL_CLASSIFY":  "01",
			"DEAL_STAGE":     "0100",
			"DEAL_PROVINCE":  "0",
			"DEAL_CITY":      "0",
			"DEAL_PLATFORM":  "0",
			"BID_PLATFORM":   "0",
			"DEAL_TRADE":     "0",
			"isShowAll":      "1",
			"PAGENUMBER":     "2",
			"FINDTXT":        "",
		},
	})
	if err != nil {
		log.Panic(err)
	}
	jsonData,_ := resp.Json()
	lls := jsonData.Get("data").Array()
	for _, ll := range lls {
		log.Print(ll.Get("title"))
	}
}
```