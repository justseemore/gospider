package main

import (
	"testing"

	"gitee.com/baixudong/gospider/requests"
)

func TestIp(t *testing.T) {
	reqCli, err := requests.NewClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := reqCli.Request(nil, "get", "http://myip.top")
	if err != nil {
		t.Fatal(err)
	}
	jsonData := resp.Json()
	if jsonData.Get("ip").String() == "" {
		t.Fatal("没有ip")
	}
	resp, err = reqCli.Request(nil, "get", "https://myip.top")
	if err != nil {
		t.Fatal(err)
	}
	jsonData = resp.Json()
	if jsonData.Get("ip").String() == "" {
		t.Fatal("没有ip")
	}
}
