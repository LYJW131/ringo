package i18n

// The Apple Music source's text (internal/applemusic) in Simplified Chinese.
var appleMusicChinese = map[string]string{
	"About %s|Apple Music": "%s简介",
	"Apple Music didn't accept the sign-in. Sign in again.":                                         "Apple Music 不接受当前登录，请重新登录。",
	"Apple Music didn't accept the token of the wrapper's account. Log it in to the wrapper again.": "Apple Music 不接受 wrapper 里这个账户的令牌。请在 wrapper 里重新登录该账户。",
	"Apple Music didn't sign you in":                                                                "Apple Music 没有完成登录",
	"Apple Music doesn't have this":                                                                 "Apple Music 上没有这个内容",
	"Apple Music has no AAC of this song":                                                           "Apple Music 没有这首歌的 AAC 音轨",
	"Apple Music sent no key for this song":                                                         "Apple Music 没有给出这首歌的密钥",
	"Apple Music sent no license":                                                                   "Apple Music 没有返回许可证",
	"Apple Music won't play this song":                                                              "Apple Music 无法播放这首歌",
	"Artists|Apple Music":                                                                           "艺人",
	"Charts":                                                                                        "排行榜",
	"Couldn't read the Apple Music web player's token":                                              "读取不到 Apple Music 网页播放器的令牌",
	"Favorite Songs":                                                                                "喜爱歌曲",
	"New|Apple Music":                                                                               "新发现",
	"No such page":                                                                                  "没有这个页面",
	"Personal mixes you add will appear here.":                                                      "你添加的个人精选将显示在此处。",
	"Radio|Apple Music":                                                                             "广播",
	"Recently Added":                                                                                "最近添加",
	"Songs you uploaded can't be added back once deleted, so this album stays in your library": "你上传的歌曲删除后无法再添加回来，所以这张专辑会保留在资料库中",
	"That isn't a media-user-token: paste the value of the media-user-token cookie":            "这不是 media-user-token：请粘贴 Cookie 中 media-user-token 的值",
	"The playlist changed elsewhere. Try again in a moment.":                                   "此播放列表已在别处更改，请稍后再试。",
	"There are no items in your library.":                                                      "你的资料库中没有任何项目。",
	"This song isn't available in your country or region":                                      "你所在的国家或地区不提供这首歌",
	"Account in %s": "%s 的账户",
	"An account the wrapper holds can't be removed here":                                     "wrapper 里的账户不能在这里移除",
	"An account the wrapper holds can't be signed out of: switch to another account instead": "wrapper 里的账户不能退出登录，请切换到其他账户",
	"Apple Music didn't accept this Media User Token. Copy a new one and try again.":         "Apple Music 不接受这个 Media User Token。请复制一个新的再试。",
	"Apple Music didn't accept this account's sign-in. Add it again.":                        "Apple Music 不接受这个账户的登录，请重新添加它。",
	"Set the Wrapper Address in the settings to use the wrapper's accounts":                  "要使用 wrapper 里的账户，请先在设置里填写 Wrapper 地址",
	"The wrapper doesn't hold this account anymore, or it isn't ready":                       "wrapper 里已经没有这个账户，或者它还没有就绪",
	"This account isn't here anymore":                                                        "这个账户已经不在了",
}

func init() {
	for en, zh := range appleMusicChinese {
		zhCN[en] = zh
	}
}
