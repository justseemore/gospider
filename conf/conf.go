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
func GetTempDirPath() (string, error) {
	dir, err := GetMainDirPath()
	if err != nil {
		return "", err
	}
	tempDir := tools.PathJoin(dir, "TempDir")
	if !tools.PathExist(tempDir) {
		return tempDir, os.MkdirAll(tempDir, 0777)
	}
	return tempDir, nil
}

// 浏览器需要删除的临时目录
func GetTempChromeDirPath() (string, error) {
	dir, err := GetTempDirPath()
	if err != nil {
		return "", err
	}
	return os.MkdirTemp(dir, "chrome")
}
