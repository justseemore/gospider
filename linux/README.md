# 这是一个远程操控Linux 的模块包括文件的上传下载，linux 执行命令
~~~
func main() {
	cli, err := linux.NewClient(linux.ClientOption{Host: "192.168.1.30", Port: 22, Usr: "py_baixudong", Pwd: "jhcms001"})
	if err != nil {
		log.Panic(err)
	}
	defer cli.Close()
	scr, err := cli.NewScreen("proxy")
	if err != nil {
		log.Panic(err)
	}
	defer scr.Close()
	log.Print(scr.IsRun)
	rs, err := scr.SudoRun([]byte("sudo ./proxy30\n"))
	if err != nil {
		log.Panic(err)
	}
	log.Print(string(rs))

	// log.Print(cli)

}
~~~

