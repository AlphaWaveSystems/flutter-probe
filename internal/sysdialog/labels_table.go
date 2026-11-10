package sysdialog

import "sort"

// iosRoleLabels maps a BCP-47-ish language tag to the button labels iOS renders
// for the standard system permission alerts, keyed by role:
//
//	allow              "Allow" (notification, camera, microphone, reminders,
//	                   speech recognition, motion & fitness)
//	deny               "Don't Allow" (every alert above plus the location alert;
//	                   several spellings exist in some languages)
//	allow_once         location alert, first button
//	allow_while_using  location alert, second button
//
// The data was MEASURED, not translated by hand: for each language the iPhone
// 17e simulator (iOS 26.3 runtime) was switched to that language and locale,
// the alerts were triggered by a throwaway app, and the button labels were read
// back with the system-dialog driver. Measured on 2026-10-09.
//
// Roles that could not be observed on the simulator (ok, cancel, open) are
// deliberately absent rather than guessed. Languages not listed were not
// measured. Labels keep the exact characters iOS returned (for example U+2019
// apostrophes); lookups go through normalize, which folds them.
var iosRoleLabels = map[string]map[string][]string{
	"en": {
		"allow":             {"Allow"},
		"deny":              {"Don’t Allow"},
		"allow_once":        {"Allow Once"},
		"allow_while_using": {"Allow While Using App"},
	},
	"de": {
		"allow":             {"Erlauben"},
		"deny":              {"Nicht erlauben"},
		"allow_once":        {"Einmal erlauben"},
		"allow_while_using": {"Beim Verwenden der App erlauben"},
	},
	"fr": {
		"allow":             {"Autoriser"},
		"deny":              {"Refuser", "Ne pas autoriser"},
		"allow_once":        {"Autoriser une fois"},
		"allow_while_using": {"Autoriser lorsque l’app est active"},
	},
	"es": {
		"allow":             {"Permitir"},
		"deny":              {"No permitir"},
		"allow_once":        {"Permitir una vez"},
		"allow_while_using": {"Permitir al usarse la app"},
	},
	"it": {
		"allow":             {"Consenti"},
		"deny":              {"Non consentire"},
		"allow_once":        {"Consenti una volta"},
		"allow_while_using": {"Consenti quando utilizzi l’app"},
	},
	"pt-BR": {
		"allow":             {"Permitir"},
		"deny":              {"Não Permitir"},
		"allow_once":        {"Permitir Uma Vez"},
		"allow_while_using": {"Permitir Durante o Uso do App"},
	},
	"nl": {
		"allow":             {"Sta toe"},
		"deny":              {"Sta niet toe"},
		"allow_once":        {"Sta één sessie toe"},
		"allow_while_using": {"Bij gebruik van app"},
	},
	"sv": {
		"allow":             {"Tillåt"},
		"deny":              {"Tillåt inte"},
		"allow_once":        {"Tillåt en gång"},
		"allow_while_using": {"Tillåt medan appen används"},
	},
	"da": {
		"allow":             {"Tillad"},
		"deny":              {"Tillad ikke"},
		"allow_once":        {"Tillad en gang"},
		"allow_while_using": {"Tillad, mens du bruger app"},
	},
	"nb": {
		"allow":             {"Tillat"},
		"deny":              {"Ikke tillat"},
		"allow_once":        {"Tillat én gang"},
		"allow_while_using": {"Tillat mens appen er i bruk"},
	},
	"fi": {
		"allow":             {"Salli"},
		"deny":              {"Älä salli"},
		"allow_once":        {"Salli kerran"},
		"allow_while_using": {"Salli käytettäessä"},
	},
	"pl": {
		"allow":             {"Pozwalaj", "Pozwól"},
		"deny":              {"Nie pozwalaj"},
		"allow_once":        {"Pozwól raz"},
		"allow_while_using": {"Pozwalaj, gdy używam aplikacji"},
	},
	"cs": {
		"allow":             {"Povolit"},
		"deny":              {"Zakázat", "Nepovolovat"},
		"allow_once":        {"Povolit jednou"},
		"allow_while_using": {"Povolit jen při používání aplikace"},
	},
	"tr": {
		"allow":             {"İzin Ver"},
		"deny":              {"İzin Verme"},
		"allow_once":        {"Bir Kez İzin Ver"},
		"allow_while_using": {"Uygulamayı Kullanırken İzin Ver"},
	},
	"ru": {
		"allow":             {"Разрешить"},
		"deny":              {"Не разрешать", "Запретить"},
		"allow_once":        {"Однократно"},
		"allow_while_using": {"При использовании приложения"},
	},
	"uk": {
		"allow":             {"Дозволити"},
		"deny":              {"Заборонити", "Не дозволяти"},
		"allow_once":        {"Дозволити один раз"},
		"allow_while_using": {"Дозволяти за використання"},
	},
	"el": {
		"allow":             {"Να επιτρέπεται", "Να επιτραπεί"},
		"deny":              {"Να μην επιτρέπεται", "Να μην επιτραπεί", "Όχι"},
		"allow_once":        {"Να επιτραπεί μία φορά"},
		"allow_while_using": {"Κατά τη χρήση της εφαρμογής"},
	},
	"he": {
		"allow":             {"אישור"},
		"deny":              {"סירוב"},
		"allow_once":        {"פעם אחת"},
		"allow_while_using": {"בעת השימוש ביישום"},
	},
	"ar": {
		"allow":             {"السماح"},
		"deny":              {"عدم السماح"},
		"allow_once":        {"السماح لمرة واحدة"},
		"allow_while_using": {"السماح أثناء استخدام التطبيق"},
	},
	"hi": {
		"allow":             {"अनुमति दें"},
		"deny":              {"अनुमति न दें"},
		"allow_once":        {"एक बार अनुमति दें"},
		"allow_while_using": {"ऐप उपयोग के दौरान अनुमति दें"},
	},
	"th": {
		"allow":             {"อนุญาต"},
		"deny":              {"ไม่อนุญาต"},
		"allow_once":        {"อนุญาตครั้งเดียว"},
		"allow_while_using": {"อนุญาตในระหว่างใช้งานแอป"},
	},
	"vi": {
		"allow":             {"Cho phép"},
		"deny":              {"Từ chối"},
		"allow_once":        {"Cho phép một lần"},
		"allow_while_using": {"Cho phép khi dùng ứng dụng"},
	},
	"id": {
		"allow":             {"Izinkan"},
		"deny":              {"Jangan Izinkan"},
		"allow_once":        {"Izinkan Sekali"},
		"allow_while_using": {"Izinkan Saat Menggunakan App"},
	},
	"ja": {
		"allow":             {"許可"},
		"deny":              {"許可しない"},
		"allow_once":        {"1度だけ許可"},
		"allow_while_using": {"アプリの使用中は許可"},
	},
	"ko": {
		"allow":             {"허용"},
		"deny":              {"허용 안 함"},
		"allow_once":        {"한 번 허용"},
		"allow_while_using": {"앱을 사용하는 동안 허용"},
	},
	"zh-Hans": {
		"allow":             {"允许"},
		"deny":              {"不允许"},
		"allow_once":        {"允许一次"},
		"allow_while_using": {"使用App时允许"},
	},
	"zh-Hant": {
		"allow":             {"允許"},
		"deny":              {"不允許"},
		"allow_once":        {"允許一次"},
		"allow_while_using": {"使用App期間允許"},
	},
}

// iosSystemLabels holds the buttons of the standard iOS system alerts, per locale: the permission alerts (allow and
// deny), the App Tracking Transparency alert (its decline button is "Ask App Not to Track"), and the location alert
// (allow once and allow while using). The strings are read from the system's own localization in the iOS 26.5
// simulator runtime (TCC.framework and the CoreLocation prompt plugin), not from a device run. The measured table above
// agrees with them for the 27 languages it covers, which the tests check. Only dismiss and button matching use these
// labels; RoleLabels still lists the measured ones alone.
var iosSystemLabels = map[string]map[string][]string{
	"ar": {
		"allow":             {"السماح"},
		"deny":              {"عدم السماح", "مطالبة التطبيق بعدم التتبع"},
		"allow_once":        {"السماح لمرة واحدة"},
		"allow_while_using": {"السماح أثناء استخدام التطبيق"},
	},
	"bg": {
		"allow":             {"Позволи"},
		"deny":              {"Не позволявай", "Заяви да не проследява"},
		"allow_once":        {"Позволи веднъж"},
		"allow_while_using": {"Позволи при използване"},
	},
	"bn": {
		"allow":             {"অনুমতি দিন"},
		"deny":              {"অনুমতি দেবেন না", "অ্যাপকে ট্র্যাক করার অনুমতি দেবেন না"},
		"allow_once":        {"একবার অনুমতি দিন"},
		"allow_while_using": {"অ্যাপ ব্যবহার করার সময় অনুমতি দিন"},
	},
	"ca": {
		"allow":             {"Permet"},
		"deny":              {"No permetis", "Demana a l’app que no em rastregi"},
		"allow_once":        {"Permet una vegada"},
		"allow_while_using": {"Permet amb l’app en ús"},
	},
	"cs": {
		"allow":             {"Povolit"},
		"deny":              {"Nepovolovat", "Požádat aplikaci o nesledování", "Zakázat"},
		"allow_once":        {"Povolit jednou"},
		"allow_while_using": {"Povolit jen při používání aplikace"},
	},
	"da": {
		"allow":             {"Tillad"},
		"deny":              {"Tillad ikke", "Bed app om ikke at spore"},
		"allow_once":        {"Tillad en gang"},
		"allow_while_using": {"Tillad, mens du bruger app"},
	},
	"de": {
		"allow":             {"Erlauben"},
		"deny":              {"Nicht erlauben", "App-Tracking ablehnen"},
		"allow_once":        {"Einmal erlauben"},
		"allow_while_using": {"Beim Verwenden der App erlauben"},
	},
	"el": {
		"allow":             {"Να επιτραπεί"},
		"deny":              {"Να μην επιτραπεί", "Άρνηση", "Αίτημα στην εφαρμογή για μη ανίχνευση", "Όχι"},
		"allow_once":        {"Να επιτραπεί μία φορά"},
		"allow_while_using": {"Κατά τη χρήση της εφαρμογής"},
	},
	"en": {
		"allow":             {"Allow"},
		"deny":              {"Don’t Allow", "Don't Allow", "Ask App Not to Track"},
		"allow_once":        {"Allow Once"},
		"allow_while_using": {"Allow While Using App"},
	},
	"en_AU": {
		"allow":             {"Allow"},
		"deny":              {"Don’t Allow", "Ask App Not to Track"},
		"allow_once":        {"Allow Once"},
		"allow_while_using": {"Allow While Using App"},
	},
	"en_GB": {
		"allow":             {"Allow"},
		"deny":              {"Don’t Allow", "Ask App Not to Track"},
		"allow_once":        {"Allow Once"},
		"allow_while_using": {"Allow While Using App"},
	},
	"es": {
		"allow":             {"Permitir"},
		"deny":              {"No permitir", "Pedir a la app que no rastree"},
		"allow_once":        {"Permitir una vez"},
		"allow_while_using": {"Permitir al usarse la app"},
	},
	"es_419": {
		"allow":             {"Permitir"},
		"deny":              {"No permitir", "Solicitar a la app no rastrear"},
		"allow_once":        {"Permitir una vez"},
		"allow_while_using": {"Permitir al usar la app"},
	},
	"es_US": {
		"allow": {"Permitir"},
		"deny":  {"No permitir", "Solicitar a la app no rastrear"},
	},
	"fi": {
		"allow":             {"Salli"},
		"deny":              {"Älä salli", "Pyydä, ettei appi seuraa sinua"},
		"allow_once":        {"Salli kerran"},
		"allow_while_using": {"Salli käytettäessä"},
	},
	"fr": {
		"allow":             {"Autoriser"},
		"deny":              {"Ne pas autoriser", "Refuser", "Demander à l’app de ne pas me suivre"},
		"allow_once":        {"Autoriser une fois"},
		"allow_while_using": {"Autoriser lorsque l’app est active"},
	},
	"fr_CA": {
		"allow":             {"Autoriser"},
		"deny":              {"Refuser", "Demander à l’app de ne pas faire de suivi"},
		"allow_once":        {"Autoriser une fois"},
		"allow_while_using": {"Autoriser lorsque l’app est active"},
	},
	"gu": {
		"allow":             {"અનુમતિ આપો"},
		"deny":              {"અનુમતિ આપશો નહિ", "ઍપને ટ્રૅક ન કરવાનું કહો"},
		"allow_once":        {"એકવાર અનુમતિ આપો"},
		"allow_while_using": {"ઍપનો ઉપયોગ કરતી વખતે અનુમતિ આપો"},
	},
	"he": {
		"allow":             {"אישור"},
		"deny":              {"סירוב", "בקשה מהיישום לא לבצע מעקב"},
		"allow_once":        {"פעם אחת"},
		"allow_while_using": {"בעת השימוש ביישום"},
	},
	"hi": {
		"allow":             {"अनुमति दें"},
		"deny":              {"अनुमति न दें", "ऐप से ट्रैक न करने के लिए कहें"},
		"allow_once":        {"एक बार अनुमति दें"},
		"allow_while_using": {"ऐप उपयोग के दौरान अनुमति दें"},
	},
	"hr": {
		"allow":             {"Dozvoli"},
		"deny":              {"Nemoj dozvoliti", "Zatraži od aplikacije da ne prati"},
		"allow_once":        {"Dozvoli jedanput"},
		"allow_while_using": {"Dozvoli tijekom uporabe aplikacije"},
	},
	"hu": {
		"allow":             {"Engedélyezés"},
		"deny":              {"Nem engedélyezem", "Tiltás", "App megkérése, hogy mellőzze a követést"},
		"allow_once":        {"Engedélyezés egyszer"},
		"allow_while_using": {"Az app használata közben"},
	},
	"id": {
		"allow":             {"Izinkan"},
		"deny":              {"Jangan Izinkan", "Minta App Tidak Melacak"},
		"allow_once":        {"Izinkan Sekali"},
		"allow_while_using": {"Izinkan Saat Menggunakan App"},
	},
	"it": {
		"allow":             {"Consenti"},
		"deny":              {"Non consentire", "Non tracciarmi"},
		"allow_once":        {"Consenti una volta"},
		"allow_while_using": {"Consenti quando utilizzi l’app"},
	},
	"ja": {
		"allow":             {"許可"},
		"deny":              {"許可しない", "アプリにトラッキングしないように要求"},
		"allow_once":        {"1度だけ許可"},
		"allow_while_using": {"アプリの使用中は許可"},
	},
	"kk": {
		"allow":             {"Рұқсат беру"},
		"deny":              {"Рұқсат бермеу", "Қолданбадан қадағаламауды сұрау"},
		"allow_once":        {"Бір рет рұқсат беру"},
		"allow_while_using": {"Қолданбаны пайдаланғанда рұқсат беру"},
	},
	"kn": {
		"allow":             {"ಅನುಮತಿಸಿ"},
		"deny":              {"ಅನುಮತಿಸಬೇಡಿ", "ಆ್ಯಪ್‌ಗೆ ಟ್ರ್ಯಾಕ್ ಮಾಡಬೇಡಿ ಎಂದು ಹೇಳಿ"},
		"allow_once":        {"ಒಮ್ಮೆ ಅನುಮತಿಸಿ"},
		"allow_while_using": {"ಆ್ಯಪ್ ಬಳಸುವಾಗ ಅನುಮತಿಸಿ"},
	},
	"ko": {
		"allow":             {"허용"},
		"deny":              {"허용 안 함", "앱에 추적 금지 요청"},
		"allow_once":        {"한 번 허용"},
		"allow_while_using": {"앱을 사용하는 동안 허용"},
	},
	"lt": {
		"allow":             {"Leisti"},
		"deny":              {"Neleisti", "Paprašyti programos nesekti"},
		"allow_once":        {"Leisti vieną kartą"},
		"allow_while_using": {"Leisti naudojant programą"},
	},
	"ml": {
		"allow":             {"അനുവദിക്കൂ"},
		"deny":              {"അനുവദിക്കേണ്ട", "ട്രാക്ക് ചെയ്യരുതെന്ന് ആപ്പിനോട് ആവശ്യപ്പെടൂ"},
		"allow_once":        {"ഒരു തവണ അനുവദിക്കൂ"},
		"allow_while_using": {"ആപ്പ് ഉപയോഗിക്കുമ്പോൾ മാത്രം"},
	},
	"mr": {
		"allow":             {"अनुमती द्या"},
		"deny":              {"अनुमती देऊ नका", "ॲपना ट्रॅक करू नका असे सांगा"},
		"allow_once":        {"एकदाच अनुमती द्या"},
		"allow_while_using": {"ॲप वापरताना अनुमती द्या"},
	},
	"ms": {
		"allow":             {"Benarkan"},
		"deny":              {"Jangan Benarkan", "Minta App untuk Tidak Menjejak"},
		"allow_once":        {"Benarkan Sekali"},
		"allow_while_using": {"Benarkan Semasa Menggunakan App"},
	},
	"nl": {
		"allow":             {"Sta toe"},
		"deny":              {"Sta niet toe", "Weiger", "Vraag app om niet te tracken"},
		"allow_once":        {"Sta één sessie toe"},
		"allow_while_using": {"Bij gebruik van app"},
	},
	"no": {
		"allow":             {"Tillat"},
		"deny":              {"Ikke tillat", "Be om å ikke bli sporet"},
		"allow_once":        {"Tillat én gang"},
		"allow_while_using": {"Tillat mens appen er i bruk"},
	},
	"or": {
		"allow":             {"ଅନୁମତି ଦିଅନ୍ତୁ"},
		"deny":              {"ଅନୁମତି ଦିଅନ୍ତୁ ନାହିଁ", "ଟ୍ରାକ୍ ନକରିବା ପାଇଁ ଆପ୍‌କୁ କୁହନ୍ତୁ"},
		"allow_once":        {"ଥରେ ଅନୁମତି ଦିଅନ୍ତୁ"},
		"allow_while_using": {"ଆପ୍ ବ୍ଯବହାର କରୁ଼ଥିବା ବେଳେ ଅନୁମତି ଦିଅନ୍ତୁ"},
	},
	"pa": {
		"allow":             {"ਆਗਿਆ ਦਿਓ"},
		"deny":              {"ਆਗਿਆ ਨਾ ਦਿਓ", "ਐਪਾਂ ਨੂੰ ਕਹੋ ਕਿ ਟ੍ਰੈਕ ਨਾ ਕਰਨ"},
		"allow_once":        {"ਇੱਕ ਵਾਰ ਆਗਿਆ ਦਿਓ"},
		"allow_while_using": {"ਐਪ ਦੀ ਵਰਤੋਂ ਕਰਨ ਵੇਲੇ ਆਗਿਆ ਦਿਓ"},
	},
	"pl": {
		"allow":             {"Pozwól"},
		"deny":              {"Nie pozwalaj", "Poproś aplikację o nieśledzenie"},
		"allow_once":        {"Pozwól raz"},
		"allow_while_using": {"Pozwalaj, gdy używam aplikacji"},
	},
	"pt_BR": {
		"allow":             {"Permitir"},
		"deny":              {"Não Permitir", "Pedir ao App para Não Rastrear"},
		"allow_once":        {"Permitir Uma Vez"},
		"allow_while_using": {"Permitir Durante o Uso do App"},
	},
	"pt_PT": {
		"allow":             {"Permitir"},
		"deny":              {"Não permitir", "Pedir à aplicação para não seguir"},
		"allow_once":        {"Permitir uma vez"},
		"allow_while_using": {"Permitir durante utilização"},
	},
	"ro": {
		"allow":             {"Permiteți"},
		"deny":              {"Nu permiteți", "Cereți aplicației să nu vă urmărească"},
		"allow_once":        {"Permiteți o dată"},
		"allow_while_using": {"Permiteți în timpul utilizării aplicației"},
	},
	"ru": {
		"allow":             {"Разрешить"},
		"deny":              {"Не разрешать", "Запретить", "Попросить не отслеживать"},
		"allow_once":        {"Однократно"},
		"allow_while_using": {"При использовании приложения"},
	},
	"sk": {
		"allow":             {"Povoliť"},
		"deny":              {"Nepovoliť", "Požiadať apku o nesledovanie"},
		"allow_once":        {"Povoliť raz"},
		"allow_while_using": {"Povoliť len pri používaní apky"},
	},
	"sl": {
		"allow":             {"Dovoli"},
		"deny":              {"Ne dovoli", "Aplikaciji ne dovoli sledenja"},
		"allow_once":        {"Dovoli tokrat"},
		"allow_while_using": {"Dovoli med uporabo aplikacije"},
	},
	"sv": {
		"allow":             {"Tillåt"},
		"deny":              {"Tillåt inte", "Begär att inte bli spårad av appen"},
		"allow_once":        {"Tillåt en gång"},
		"allow_while_using": {"Tillåt medan appen används"},
	},
	"ta": {
		"allow":             {"அனுமதி"},
		"deny":              {"அனுமதிக்காதே", "செயலி கண்காணிக்க வேண்டாம் எனக் கோருக"},
		"allow_once":        {"ஒருமுறை அனுமதி"},
		"allow_while_using": {"செயலியைப் பயன்படுத்தும்போது அனுமதி"},
	},
	"te": {
		"allow":             {"అనుమతించండి"},
		"deny":              {"అనుమతించవద్దు", "ట్ర్యాక్ చేయకూడదని యాప్‌కి చెప్పండి"},
		"allow_once":        {"ఒకసారికి అనుమతించండి"},
		"allow_while_using": {"యాప్‌ను ఉపయోగిస్తున్నప్పుడు"},
	},
	"th": {
		"allow":             {"อนุญาต"},
		"deny":              {"ไม่อนุญาต", "บอกแอปไม่ให้ติดตาม"},
		"allow_once":        {"อนุญาตครั้งเดียว"},
		"allow_while_using": {"อนุญาตในระหว่างใช้งานแอป"},
	},
	"tr": {
		"allow":             {"İzin Ver"},
		"deny":              {"İzin Verme", "Uygulama Beni Takip Etmesin"},
		"allow_once":        {"Bir Kez İzin Ver"},
		"allow_while_using": {"Uygulamayı Kullanırken İzin Ver"},
	},
	"uk": {
		"allow":             {"Дозволити"},
		"deny":              {"Заборонити", "Просити програму не стежити", "Не дозволяти"},
		"allow_once":        {"Дозволити один раз"},
		"allow_while_using": {"Дозволяти за використання"},
	},
	"ur": {
		"allow":             {"اجازت دیں"},
		"deny":              {"اجازت نہ دیں", "ایپ سے ٹریک نہ کرنے کے لیے کہیں"},
		"allow_once":        {"ایک بار اجازت دیں"},
		"allow_while_using": {"ایپ استعمال کے دوران اجازت دیں"},
	},
	"vi": {
		"allow":             {"Cho phép"},
		"deny":              {"Từ chối", "Yêu cầu ứng dụng không theo dõi"},
		"allow_once":        {"Cho phép một lần"},
		"allow_while_using": {"Cho phép khi dùng ứng dụng"},
	},
	"yue_CN": {
		"allow": {"允许"},
		"deny":  {"不允许", "要求App不跟踪"},
	},
	"zh_CN": {
		"allow":             {"允许"},
		"deny":              {"不允许", "要求App不跟踪"},
		"allow_once":        {"允许一次"},
		"allow_while_using": {"使用App时允许"},
	},
	"zh_HK": {
		"allow":             {"允許"},
		"deny":              {"不允許", "要求App不要追蹤"},
		"allow_once":        {"允許一次"},
		"allow_while_using": {"使用App時允許"},
	},
	"zh_TW": {
		"allow":             {"允許"},
		"deny":              {"不允許", "要求App不要追蹤"},
		"allow_once":        {"允許一次"},
		"allow_while_using": {"使用App期間允許"},
	},
}

// roleOrder fixes the iteration order so results are deterministic.
var roleOrder = []string{"allow", "deny", "allow_once", "allow_while_using", "ok", "cancel", "open"}

// langOrder lists "en" first, then the remaining languages alphabetically.
func langOrder() []string {
	langs := make([]string, 0, len(iosRoleLabels))
	for l := range iosRoleLabels {
		if l != "en" {
			langs = append(langs, l)
		}
	}
	sort.Strings(langs)
	if _, ok := iosRoleLabels["en"]; ok {
		langs = append([]string{"en"}, langs...)
	}
	return langs
}

// RoleLabels returns every measured label for role (allow, deny, allow_once,
// allow_while_using, ...) across all languages, deduplicated by normalized
// form, English first. It returns nil for a role with no measured labels.
func RoleLabels(role string) []string {
	var out []string
	seen := map[string]bool{}
	for _, lang := range langOrder() {
		for _, l := range iosRoleLabels[lang][role] {
			n := normalize(l)
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, l)
		}
	}
	return out
}

// LabelRole reports the role of a button label (compared with normalize), or
// ok=false when the label is unknown or ambiguous between roles.
func LabelRole(label string) (role string, ok bool) {
	n := normalize(label)
	if n == "" {
		return "", false
	}
	for _, r := range roleOrder {
		for _, lang := range langOrder() {
			for _, l := range iosRoleLabels[lang][r] {
				if normalize(l) != n {
					continue
				}
				if role != "" && role != r {
					return "", false
				}
				role = r
			}
		}
	}
	for _, byRole := range iosSystemLabels {
		for r, labels := range byRole {
			for _, l := range labels {
				if normalize(l) != n {
					continue
				}
				if role != "" && role != r {
					return "", false
				}
				role = r
			}
		}
	}
	return role, role != ""
}

func init() {
	extraLabelRole = func(folded string) (Role, bool) {
		r, ok := LabelRole(folded)
		return Role(r), ok
	}
}
