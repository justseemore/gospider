# 版本
```
v0.10.0
```
# 伪表头顺序
```
":method"
":authority"
":scheme"
":path"
```
# initialSettings
```
{
    "SETTINGS": {
      "1": "65536",
      "2": "0",
      "3": "1000",
      "4": "6291456",
      "6": "262144"
    },
    "WINDOW_UPDATE": "15663105",
    "HEADERS": [
      ":method",
      ":authority",
      ":scheme",
      ":path"
    ]
}
```
## 原始
```
	initialSettings := []Setting{
		{ID: SettingEnablePush, Val: 0},
		{ID: SettingInitialWindowSize, Val: transportDefaultStreamFlow},
	}
	if max := t.maxFrameReadSize(); max != 0 {
		initialSettings = append(initialSettings, Setting{ID: SettingMaxFrameSize, Val: max})
	}
	if max := t.maxHeaderListSize(); max != 0 {
		initialSettings = append(initialSettings, Setting{ID: SettingMaxHeaderListSize, Val: max})
	}
	if maxHeaderTableSize != initialHeaderTableSize {
		initialSettings = append(initialSettings, Setting{ID: SettingHeaderTableSize, Val: maxHeaderTableSize})
	}
```
## 修改后
```
	initialSettings := []Setting{}
	initialSettings = append(initialSettings, Setting{ID: SettingHeaderTableSize, Val: maxHeaderTableSize})//1
	initialSettings = append(initialSettings, Setting{ID: SettingEnablePush, Val: 0})//2
	initialSettings = append(initialSettings, Setting{ID: SettingMaxConcurrentStreams, Val: defaultMaxStreams})//3
	initialSettings = append(initialSettings, Setting{ID: SettingInitialWindowSize, Val: transportDefaultStreamFlow})//4
	initialSettings = append(initialSettings, Setting{ID: SettingMaxHeaderListSize, Val: t.maxHeaderListSize()})//6
```
## 修改变量
```
defaultMaxStreams  //3:1000
transportDefaultStreamFlow //4:6291456
transportDefaultConnFlow //WINDOW_UPDATE:15663105
```