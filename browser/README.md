# 功能特色
* 原生chrome cdp 协议,快如闪电
* 使用强大的gospider/requests代替网络请求,使浏览器具备gospider/requests的所有特性
* 对数据进行拦截,修改,缓存
* 支持对函数进行hook和打包。
* 支持标签页代理，各个标签的代理互不干扰
* 支持高并发爬虫，同时渲染上百标签页互不干扰
* 自动无头反识别,集成各种反无头，反浏览器的各种功能，足以对抗百分之99的，浏览器识别
* 支持修改浏览器的ja3指纹
  
# 修改浏览器的ja3指纹
```go
func main() {
	browCli, err := browser.NewClient(nil, browser.ClientOption{
		Headless: true,
	})
	if err != nil {
		log.Panic(err)
	}
	defer browCli.Close()

	ja3Str := "772,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,0-23-65281-10-11-35-16-5-13-18-51-45-43-27-17513,29-23-24,0"
	Ja3Spec, err := ja3.CreateSpecWithStr(ja3Str)
	if err != nil {
		log.Panic(err)
	}
	page, err := browCli.NewPage(nil, browser.PageOption{Ja3Spec: Ja3Spec})
	if err != nil {
		log.Panic(err)
	}
	defer page.Close()
	err = page.GoTo(nil, "https://tools.scrapfly.io/api/fp/ja3?extended=1")
	if err != nil {
		log.Panic(err)
	}
	html, err := page.Html(nil)
	if err != nil {
		log.Panic(err)
	}
	log.Print(html.Text())
	// <-page.Done()
}
```







