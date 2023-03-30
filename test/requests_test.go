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

func TestJa3(t *testing.T) {
	reqCli, err := requests.NewClient(nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := reqCli.Request(nil, "get", "https://tools.scrapfly.io/api/fp/ja3?extended=1")
	if err != nil {
		t.Fatal(err)
	}
	jsonData := resp.Json()
	goJa3Str := jsonData.Get("ja3").String()
	if goJa3Str == "" {
		t.Fatal("没有ja3")
	}
	resp, err = reqCli.Request(nil, "get", "https://tools.scrapfly.io/api/fp/ja3?extended=1", requests.RequestOption{Ja3: true})
	if err != nil {
		t.Fatal(err)
	}
	jsonData = resp.Json()
	chromeJa3Str := jsonData.Get("ja3").String()
	if chromeJa3Str == "" {
		t.Fatal("没有ja3")
	}
	if chromeJa3Str == goJa3Str {
		t.Fatal("ja3 更改失败")
	}
}
