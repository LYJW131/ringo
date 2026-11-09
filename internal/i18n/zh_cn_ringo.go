package i18n

// Ringo's own text, apart from MyGo Music's catalog for that to keep merging.
// Apple's things are called what Apple Music calls them in Simplified Chinese.

// reworded is what MyGo Music's English says of plugins, of which Ringo has
// one that the listener needn't know as such, said of Apple Music instead.
// T reads it; ringoChinese has the same texts' Chinese.
var reworded = map[string]string{
	"Adjust each song's volume by what its plugin measured of it, for them to sound as loud as each other": "Adjust each song's volume by how loud it was measured to be, for them to sound as loud as each other",
	"Couldn't reach this plugin":                                        "Couldn't reach Apple Music",
	"The plugin needs setting up":                                       "Apple Music needs setting up",
	"What's asked of plugins that have a choice, from the next song on": "From the next song on. Lossless plays through a wrapper, set up under Apple Music, and songs play as AAC without one",
	"You'll finish signing in with the plugin, on a page it opens.":     "You'll sign in on Apple's own page, in a window that opens.",
	"%s needs MyGo Music %s or later. Update MyGo Music first.":         "%s needs what came with MyGo Music %s. Update Ringo first.",
}

var ringoChinese = map[string]string{
	"Apple Account":                       "Apple 账户",
	"Apple Music":                         "Apple Music",
	"Apple Music Component %s":            "Apple Music 组件 %s",
	"Couldn't get what plays Apple Music": "没能取得播放 Apple Music 的组件",
	"Getting ready to play Apple Music…":  "正在准备播放 Apple Music…",
	"Not signed in":                       "未登录",
	"Use Listening History":               "使用收听历史记录",
	"Songs you play here show in Recently Played and count toward Replay and recommendations, as in Apple's own apps":      "在这里播放的歌曲会显示在“最近播放”中，并计入 Replay 和推荐，与 Apple 自己的 App 一样",
	"The catalog, your library and playback, from its own repository. It updates on its own schedule, apart from the app.": "负责目录、资料库和播放，来自它自己的仓库，更新节奏独立于 App。",

	"Adjust each song's volume by what its plugin measured of it, for them to sound as loud as each other": "按测得的响度调整每首歌的音量，让歌曲听起来一样响",
	"Couldn't reach this plugin":                                        "无法连接 Apple Music",
	"The plugin needs setting up":                                       "Apple Music 需要先进行设置",
	"What's asked of plugins that have a choice, from the next song on": "下一首歌曲开始生效。无损通过 wrapper 播放，在“Apple Music”里设置；没有 wrapper 时以 AAC 播放",
	"You'll finish signing in with the plugin, on a page it opens.":     "你将在随后打开的 Apple 登录页面中完成登录。",
	"%s needs MyGo Music %s or later. Update MyGo Music first.":         "%s 需要 MyGo Music %s 带来的功能，请先更新 Ringo。",

	// The sidebar and pages as Apple Music names them, over the names MyGo
	// Music's catalog gives some of them.
	"Home":          "主页",
	"New":           "新发现",
	"Radio":         "广播",
	"Library":       "资料库",
	"Artists":       "艺人",
	"Made for You":  "专属推荐",
	"Playlists":     "播放列表",
	"All Playlists": "所有播放列表",
	"Explicit":      "儿童不宜",
	"No items":      "无项目",
	"Previous Page": "上一页",
	"Next Page":     "下一页",
}

func init() {
	for en, zh := range ringoChinese {
		zhCN[en] = zh
	}
}
