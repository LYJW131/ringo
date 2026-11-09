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
	"For an Apple Account that signs in with a passkey or security key, which the sign-in window can't use: sign in at music.apple.com in your browser, and paste the value of its media-user-token cookie here. Empty it to sign out, or to sign in with the window again.": "用于以通行密钥或安全密钥登录的 Apple 账户（登录窗口无法使用它们）：在浏览器里登录 music.apple.com，把 Cookie 中 media-user-token 的值粘贴到这里。清空即退出登录，或改回用登录窗口登录。",
	"For lossless: the wrapper-manager that holds your Apple Music account, such as http://127.0.0.1:8080. Songs play lossless when the quality is Lossless, and as AAC without a wrapper.":                                                                                  "用于无损音频：登录了你的 Apple Music 账户的 wrapper-manager，例如 http://127.0.0.1:8080。音质设为无损时以无损播放；没有 wrapper 时播放 AAC。",
	"Library Albums":   "资料库专辑",
	"Media User Token": "Media User Token",
	"New|Apple Music":  "新发现",
	"No such page":     "没有这个页面",
	"Only songs in the Apple Music catalog play": "只能播放 Apple Music 曲库里的歌曲",
	"Radio|Apple Music":                          "广播",
	"Recently Added":                             "最近添加",
	"Set the Wrapper Address too, to sign in with an account the wrapper holds":                                                                                                               "要用 wrapper 里的账户登录，还需要设置 Wrapper 地址",
	"That isn't a media-user-token: paste the value of the media-user-token cookie":                                                                                                           "这不是 media-user-token：请粘贴 Cookie 中 media-user-token 的值",
	"The wrapper's key (API_KEY), the same one amdl uses: for a wrapper-manager on Cloudflare, or one that faces the internet. Empty for one on this computer or your network without a key.": "wrapper 的密钥（API_KEY），和 amdl 里填的是同一个：用于部署在 Cloudflare 上的 wrapper-manager，或暴露到公网的 wrapper-manager。本机或局域网里没有设密钥的留空。",
	"This song isn't available in your country or region":                                                                                                                                     "你所在的国家或地区不提供这首歌",
	"To sign in as an Apple Account the wrapper holds, without the sign-in window: its Apple ID as logged in to the wrapper, or a country or region code such as jp for the wrapper's account there. When the wrapper holds several accounts, each listener names their own. Empty to sign in here.": "用 wrapper 里登录的 Apple 账户登录，不用登录窗口：填它在 wrapper 里登录的 Apple ID，或填国家或地区代码（如 jp）以使用 wrapper 里该地区的账户。wrapper 里有多个账户时，每位听众填自己的。留空则在这里登录。",
	"Wrapper Account": "Wrapper 账户",
	"Wrapper Address": "Wrapper 地址",
	"Wrapper Key":     "Wrapper 密钥",
	"Your Station":    "你的电台",
}

func init() {
	for en, zh := range appleMusicChinese {
		zhCN[en] = zh
	}
}
