# 伪表头
```
m,a,s,p
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
## raw
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
## after
```
	initialSettings = []Setting{
		{ID: SettingHeaderTableSize, Val: initialHeaderTableSize},
		{ID: SettingEnablePush, Val: 0},
		{ID: SettingMaxConcurrentStreams, Val: defaultMaxStreams},
		{ID: SettingInitialWindowSize, Val: transportDefaultStreamFlow},
		{ID: SettingMaxHeaderListSize, Val: t.maxHeaderListSize()},
	}
```
## edit
```
initialHeaderTableSize //65536
defaultMaxStreams  //1000
transportDefaultStreamFlow //6291456
(t *Transport) maxHeaderListSize()   //262144
transportDefaultConnFlow //15663105
```