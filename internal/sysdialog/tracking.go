package sysdialog

// iosATTDecline is the decline button of the App Tracking Transparency alert in each of the 55 iOS locales. Each value
// is the REQUEST_ACCESS_DENY_kTCCServiceUserTracking string of TCC.framework in the iOS 26.5 runtime, read with plutil
// (not from a device run). The alert's allow button is an ordinary "allow" label, listed in iosSystemLabels.
var iosATTDecline = map[string]string{
	"ar":     "مطالبة التطبيق بعدم التتبع",
	"bg":     "Заяви да не проследява",
	"bn":     "অ্যাপকে ট্র্যাক করার অনুমতি দেবেন না",
	"ca":     "Demana a l’app que no em rastregi",
	"cs":     "Požádat aplikaci o\u00a0nesledování",
	"da":     "Bed app om ikke at spore",
	"de":     "App-Tracking ablehnen",
	"el":     "Αίτημα στην εφαρμογή για μη ανίχνευση",
	"en":     "Ask App Not to Track",
	"en_AU":  "Ask App Not to Track",
	"en_GB":  "Ask App Not to Track",
	"es":     "Pedir a la app que no rastree",
	"es_419": "Solicitar a la app no rastrear",
	"es_US":  "Solicitar a la app no rastrear",
	"fi":     "Pyydä, ettei appi seuraa sinua",
	"fr":     "Demander à l’app de ne pas me suivre",
	"fr_CA":  "Demander à l’app de ne pas faire de suivi",
	"gu":     "ઍપને ટ્રૅક ન કરવાનું કહો",
	"he":     "בקשה מהיישום לא לבצע מעקב",
	"hi":     "ऐप से ट्रैक न करने के लिए कहें",
	"hr":     "Zatraži od aplikacije da ne prati",
	"hu":     "App megkérése, hogy mellőzze a követést",
	"id":     "Minta App Tidak Melacak",
	"it":     "Non tracciarmi",
	"ja":     "アプリにトラッキングしないように要求",
	"kk":     "Қолданбадан қадағаламауды сұрау",
	"kn":     "ಆ್ಯಪ್\u200cಗೆ ಟ್ರ್ಯಾಕ್ ಮಾಡಬೇಡಿ ಎಂದು ಹೇಳಿ",
	"ko":     "앱에 추적 금지 요청",
	"lt":     "Paprašyti programos nesekti",
	"ml":     "ട്രാക്ക് ചെയ്യരുതെന്ന് ആപ്പിനോട് ആവശ്യപ്പെടൂ",
	"mr":     "ॲपना ट्रॅक करू नका असे सांगा",
	"ms":     "Minta App untuk Tidak Menjejak",
	"nl":     "Vraag app om niet te tracken",
	"no":     "Be om å ikke bli sporet",
	"or":     "ଟ୍ରାକ୍ ନକରିବା ପାଇଁ ଆପ୍\u200cକୁ କୁହନ୍ତୁ",
	"pa":     "ਐਪਾਂ ਨੂੰ ਕਹੋ ਕਿ ਟ੍ਰੈਕ ਨਾ ਕਰਨ",
	"pl":     "Poproś aplikację o\u00a0nieśledzenie",
	"pt_BR":  "Pedir ao App para Não Rastrear",
	"pt_PT":  "Pedir à aplicação para não seguir",
	"ro":     "Cereți aplicației să nu vă urmărească",
	"ru":     "Попросить не\u00a0отслеживать",
	"sk":     "Požiadať apku o\u00a0nesledovanie",
	"sl":     "Aplikaciji ne dovoli sledenja",
	"sv":     "Begär att inte bli spårad av appen",
	"ta":     "செயலி கண்காணிக்க வேண்டாம் எனக் கோருக",
	"te":     "ట్ర్యాక్ చేయకూడదని యాప్\u200cకి చెప్పండి",
	"th":     "บอกแอปไม่ให้ติดตาม",
	"tr":     "Uygulama Beni Takip Etmesin",
	"uk":     "Просити програму не стежити",
	"ur":     "ایپ سے ٹریک نہ کرنے کے لیے کہیں",
	"vi":     "Yêu cầu ứng dụng không theo dõi",
	"yue_CN": "要求App不跟踪",
	"zh_CN":  "要求App不跟踪",
	"zh_HK":  "要求App不要追蹤",
	"zh_TW":  "要求App不要追蹤",
}

// trackingDeclines is iosATTDecline as a set of folded labels.
var trackingDeclines = func() map[string]bool {
	set := make(map[string]bool, len(iosATTDecline))
	for _, label := range iosATTDecline {
		set[normalize(label)] = true
	}
	return set
}()

// IsTrackingDecline reports whether label is the decline button of the App Tracking Transparency alert in one of the
// iOS locales. That button has the deny role, like "Don't Allow" on a permission alert, so role matching alone cannot
// tell the two alerts apart. IsTrackingPrompt can.
func IsTrackingDecline(label string) bool {
	return trackingDeclines[normalize(label)]
}

// IsTrackingPrompt reports whether d is the App Tracking Transparency alert: one of its buttons is a tracking decline.
func (d Dialog) IsTrackingPrompt() bool {
	for _, label := range d.Buttons {
		if IsTrackingDecline(label) {
			return true
		}
	}
	return false
}
