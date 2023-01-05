package spider

import (
	"context"
	"errors"
	"fmt"
	"log"

	"gitee.com/baixudong/gospider/blog"
	"gitee.com/baixudong/gospider/browser"
	"gitee.com/baixudong/gospider/extract"
	"gitee.com/baixudong/gospider/input"
	"gitee.com/baixudong/gospider/requests"
)

type parseClient struct {
	brow   *browser.Client
	reqCli *requests.Client
	ctx    context.Context
}
type ParseResult struct{}

func Parse(ctx context.Context, url string) (parseClient, error) {
	var result parseClient
	browCli, err := browser.NewClient(ctx, browser.ClientOption{Headless: true})
	if err != nil {
		return result, err
	}
	reqCli, err := requests.NewClient(ctx)
	if err != nil {
		return result, err
	}
	client := &parseClient{
		brow:   browCli,
		reqCli: reqCli,
	}
	client.parseList(url)
	return result, nil
}

func (obj *parseClient) listOption(isBrow bool, lls [][]*extract.NodeA) ([]*extract.NodeA, error) {
	var nns []*extract.NodeA
	pn := 0
	ln := len(lls)
	isP := true
	var bbs string
	if isBrow {
		bbs = "浏览器解析>>"
	}
	for {
		if isP {
			for _, ll := range lls[pn] {
				fmt.Print(blog.Color(2, 0, ll.Time))
				fmt.Print(blog.Color(4, 0, ll.Href))
				fmt.Print(blog.Color(3, 0, ll.Title, "\r\n"))
			}
		}
		rs, err := input.Option(fmt.Sprintf("%s总列表数:%d", bbs, ln), bbs+"请选择解析正确的列表: ", map[string]string{
			"0":  bbs + "解析正确",
			"1":  "下一个列表",
			"2":  "上一个列表",
			"-1": "没有符合条件的",
		}, "0")
		if err != nil {
			return nil, err
		}
		switch rs {
		case "1":
			if pn+1 >= ln {
				log.Print(blog.Color(1, 0, "没有下一页"))
				isP = false
			} else {
				pn++
				isP = true
			}
		case "2":
			if pn-1 < 0 {
				log.Print(blog.Color(1, 0, "没有上一页"))
				isP = false
			} else {
				pn--
				isP = true
			}
		case "-1":
			return nil, errors.New("列表解析不正确")
		case "0":
			nns = lls[pn]
			return nns, nil
		}
	}
}

func (obj *parseClient) parseList(url string) error {
	rs, err := obj.reqCli.Request(obj.ctx, "get", url)
	if err != nil {
		return err
	}
	lls := extract.Lis(rs.Html())
	var nodeAs []*extract.NodeA
	if len(lls) == 0 {
		nodeAs, err = obj.parseListBrow(url)
	} else {
		nodeAs, err = obj.listOption(false, lls)
	}
	if err != nil {
		return err
	}
	log.Print(nodeAs)
	return nil
}

func (obj *parseClient) parseListBrow(url string) ([]*extract.NodeA, error) {
	page, err := obj.brow.NewPage(obj.ctx)
	if err != nil {
		return nil, err
	}
	defer page.Close()
	if err = page.GoTo(obj.ctx, url); err != nil {
		return nil, err
	}
	html, err := page.Html(obj.ctx)
	if err != nil {
		return nil, err
	}
	lls := extract.Lis(html)
	if len(lls) == 0 {
		return nil, errors.New("浏览器没有解析出列表")
	}
	return obj.listOption(true, lls)
}
