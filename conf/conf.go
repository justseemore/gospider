package conf

import (
	"os"

	"gitee.com/baixudong/gospider/tools"
)

// 脚本文件存放目录
func GetMainDirPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := tools.PathJoin(homeDir, "goSpiderMainDir")
	if !tools.PathExist(dir) {
		return dir, os.MkdirAll(dir, 0777)
	}
	return dir, nil
}

var TempChromeDir = "goBrowser"

// 浏览器需要删除的临时目录
func GetTempChromeDirPath() string {
	return tools.PathJoin(os.TempDir(), TempChromeDir)
}
