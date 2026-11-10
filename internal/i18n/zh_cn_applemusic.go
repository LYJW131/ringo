package i18n

// The Apple Music source's text (internal/applemusic) in Simplified Chinese.
var appleMusicChinese = map[string]string{
	"About %s|Apple Music": "%s简介",
	"Apple Music didn't accept the Media User Token in the settings. Paste a new one, or empty it to sign in here.": "Apple Music 不接受设置里的 Media User Token。请粘贴一个新的，或清空后在这里登录。",
	"Apple Music didn't accept the sign-in. Sign in again.":                                                         "Apple Music 不接受当前登录，请重新登录。",
	"Apple Music didn't accept the token of the wrapper's account. Log it in to the wrapper again.":                 "Apple Music 不接受 wrapper 里这个账户的令牌。请在 wrapper 里重新登录该账户。",
	"Apple Music didn't sign you in":                   "Apple Music 没有完成登录",
	"Apple Music doesn't have this":                    "Apple Music 上没有这个内容",
	"Apple Music has no AAC of this song":              "Apple Music 没有这首歌的 AAC 音轨",
	"Apple Music sent no key for this song":            "Apple Music 没有给出这首歌的密钥",
	"Apple Music sent no license":                      "Apple Music 没有返回许可证",
	"Apple Music won't play this song":                 "Apple Music 无法播放这首歌",
	"Artists|Apple Music":                              "艺人",
	"Charts":                                           "排行榜",
	"Check the Wrapper Account in the settings":        "请检查设置里的 Wrapper 账户",
	"Couldn't read the Apple Music web player's token": "读取不到 Apple Music 网页播放器的令牌",
	"Favorite Songs":                                   "喜爱歌曲",
	"New|Apple Music":                                  "新发现",
	"No such page":                                     "没有这个页面",
	"Personal mixes you add will appear here.":         "你添加的个人精选将显示在此处。",
	"Radio|Apple Music":                                "广播",
	"Recently Added":                                   "最近添加",
	"Set the Wrapper Address too, to sign in with an account the wrapper holds":                "要用 wrapper 里的账户登录，还需要设置 Wrapper 地址",
	"Songs you uploaded can't be added back once deleted, so this album stays in your library": "你上传的歌曲删除后无法再添加回来，所以这张专辑会保留在资料库中",
	"That isn't a media-user-token: paste the value of the media-user-token cookie":            "这不是 media-user-token：请粘贴 Cookie 中 media-user-token 的值",
	"There are no items in your library.":                                                      "你的资料库中没有任何项目。",
	"This song isn't available in your country or region":                                      "你所在的国家或地区不提供这首歌",
	"Your Station": "你的电台",
}

func init() {
	for en, zh := range appleMusicChinese {
		zhCN[en] = zh
	}
}
