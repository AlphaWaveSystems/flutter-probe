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
// back with the system-dialog driver. Measured on 2026-10-09; the raw output
// lives in docs/evidence/i18n-ios-dialog-labels-2026-10/.
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
	return role, role != ""
}

func init() {
	extraLabelRole = func(folded string) (Role, bool) {
		r, ok := LabelRole(folded)
		return Role(r), ok
	}
}
