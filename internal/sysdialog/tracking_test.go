package sysdialog

import (
	"slices"
	"strings"
	"testing"
)

// attCases are the App Tracking Transparency alert's buttons per iOS locale, as TCC.framework of the iOS 26.5 runtime
// has them (REQUEST_ACCESS_ALLOW_kTCCServiceUserTracking and REQUEST_ACCESS_DENY_kTCCServiceUserTracking). They are
// literals on purpose, so that a typo in iosATTDecline or iosSystemLabels cannot pass a check against itself.
var attCases = []struct{ locale, allow, decline string }{
	{"ar", "السماح", "مطالبة التطبيق بعدم التتبع"},
	{"bg", "Позволи", "Заяви да не проследява"},
	{"bn", "অনুমতি দিন", "অ্যাপকে ট্র্যাক করার অনুমতি দেবেন না"},
	{"ca", "Permet", "Demana a l’app que no em rastregi"},
	{"cs", "Povolit", "Požádat aplikaci o\u00a0nesledování"},
	{"da", "Tillad", "Bed app om ikke at spore"},
	{"de", "Erlauben", "App-Tracking ablehnen"},
	{"el", "Να επιτραπεί", "Αίτημα στην εφαρμογή για μη ανίχνευση"},
	{"en", "Allow", "Ask App Not to Track"},
	{"en_AU", "Allow", "Ask App Not to Track"},
	{"en_GB", "Allow", "Ask App Not to Track"},
	{"es", "Permitir", "Pedir a la app que no rastree"},
	{"es_419", "Permitir", "Solicitar a la app no rastrear"},
	{"es_US", "Permitir", "Solicitar a la app no rastrear"},
	{"fi", "Salli", "Pyydä, ettei appi seuraa sinua"},
	{"fr", "Autoriser", "Demander à l’app de ne pas me suivre"},
	{"fr_CA", "Autoriser", "Demander à l’app de ne pas faire de suivi"},
	{"gu", "અનુમતિ આપો", "ઍપને ટ્રૅક ન કરવાનું કહો"},
	{"he", "אישור", "בקשה מהיישום לא לבצע מעקב"},
	{"hi", "अनुमति दें", "ऐप से ट्रैक न करने के लिए कहें"},
	{"hr", "Dozvoli", "Zatraži od aplikacije da ne prati"},
	{"hu", "Engedélyezés", "App megkérése, hogy mellőzze a követést"},
	{"id", "Izinkan", "Minta App Tidak Melacak"},
	{"it", "Consenti", "Non tracciarmi"},
	{"ja", "許可", "アプリにトラッキングしないように要求"},
	{"kk", "Рұқсат беру", "Қолданбадан қадағаламауды сұрау"},
	{"kn", "ಅನುಮತಿಸಿ", "ಆ್ಯಪ್\u200cಗೆ ಟ್ರ್ಯಾಕ್ ಮಾಡಬೇಡಿ ಎಂದು ಹೇಳಿ"},
	{"ko", "허용", "앱에 추적 금지 요청"},
	{"lt", "Leisti", "Paprašyti programos nesekti"},
	{"ml", "അനുവദിക്കൂ", "ട്രാക്ക് ചെയ്യരുതെന്ന് ആപ്പിനോട് ആവശ്യപ്പെടൂ"},
	{"mr", "अनुमती द्या", "ॲपना ट्रॅक करू नका असे सांगा"},
	{"ms", "Benarkan", "Minta App untuk Tidak Menjejak"},
	{"nl", "Sta toe", "Vraag app om niet te tracken"},
	{"no", "Tillat", "Be om å ikke bli sporet"},
	{"or", "ଅନୁମତି ଦିଅନ୍ତୁ", "ଟ୍ରାକ୍ ନକରିବା ପାଇଁ ଆପ୍\u200cକୁ କୁହନ୍ତୁ"},
	{"pa", "ਆਗਿਆ ਦਿਓ", "ਐਪਾਂ ਨੂੰ ਕਹੋ ਕਿ ਟ੍ਰੈਕ ਨਾ ਕਰਨ"},
	{"pl", "Pozwól", "Poproś aplikację o\u00a0nieśledzenie"},
	{"pt_BR", "Permitir", "Pedir ao App para Não Rastrear"},
	{"pt_PT", "Permitir", "Pedir à aplicação para não seguir"},
	{"ro", "Permiteți", "Cereți aplicației să nu vă urmărească"},
	{"ru", "Разрешить", "Попросить не\u00a0отслеживать"},
	{"sk", "Povoliť", "Požiadať apku o\u00a0nesledovanie"},
	{"sl", "Dovoli", "Aplikaciji ne dovoli sledenja"},
	{"sv", "Tillåt", "Begär att inte bli spårad av appen"},
	{"ta", "அனுமதி", "செயலி கண்காணிக்க வேண்டாம் எனக் கோருக"},
	{"te", "అనుమతించండి", "ట్ర్యాక్ చేయకూడదని యాప్\u200cకి చెప్పండి"},
	{"th", "อนุญาต", "บอกแอปไม่ให้ติดตาม"},
	{"tr", "İzin Ver", "Uygulama Beni Takip Etmesin"},
	{"uk", "Дозволити", "Просити програму не стежити"},
	{"ur", "اجازت دیں", "ایپ سے ٹریک نہ کرنے کے لیے کہیں"},
	{"vi", "Cho phép", "Yêu cầu ứng dụng không theo dõi"},
	{"yue_CN", "允许", "要求App不跟踪"},
	{"zh_CN", "允许", "要求App不跟踪"},
	{"zh_HK", "允許", "要求App不要追蹤"},
	{"zh_TW", "允許", "要求App不要追蹤"},
}

func TestATTDeclineLabelPerLocale(t *testing.T) {
	if len(attCases) != 55 {
		t.Fatalf("expected 55 locales, got %d", len(attCases))
	}
	if len(iosATTDecline) != len(attCases) {
		t.Errorf("iosATTDecline has %d locales, want %d", len(iosATTDecline), len(attCases))
	}
	for _, c := range attCases {
		t.Run(c.locale, func(t *testing.T) {
			if got := iosATTDecline[c.locale]; got != c.decline {
				t.Errorf("iosATTDecline[%s] = %q, want %q", c.locale, got, c.decline)
			}
			if !slices.Contains(iosSystemLabels[c.locale]["deny"], c.decline) {
				t.Errorf("iosSystemLabels[%s] deny lacks the tracking decline %q", c.locale, c.decline)
			}
			if !slices.Contains(iosSystemLabels[c.locale]["allow"], c.allow) {
				t.Errorf("iosSystemLabels[%s] allow lacks %q", c.locale, c.allow)
			}
			if r, ok := RoleOf(c.decline); !ok || r != RoleDeny {
				t.Errorf("RoleOf(%q) = (%q, %v), want deny", c.decline, r, ok)
			}
		})
	}
}

// TestTrackingPromptIsNotAPermissionAlert checks the ATT alert in every locale, in both button orders, against the
// permission alerts of the same locale: a tracking decline makes a dialog a tracking prompt, and the Don't Allow
// button of a permission alert does not.
func TestTrackingPromptIsNotAPermissionAlert(t *testing.T) {
	for _, c := range attCases {
		for _, buttons := range [][]string{{c.decline, c.allow}, {c.allow, c.decline}} {
			if d := (Dialog{Buttons: buttons}); !d.IsTrackingPrompt() {
				t.Errorf("%s: %q is not a tracking prompt", c.locale, buttons)
			}
		}
		for _, deny := range iosSystemLabels[c.locale]["deny"] {
			if deny == c.decline {
				continue
			}
			if d := (Dialog{Buttons: []string{c.allow, deny}}); d.IsTrackingPrompt() {
				t.Errorf("%s: permission alert %q is taken for a tracking prompt", c.locale, deny)
			}
		}
	}
	for _, buttons := range [][]string{
		{"Allow", "Don’t Allow"},
		{"Einmal erlauben", "Beim Verwenden der App erlauben", "Nicht erlauben"},
		{"Allow Once", "Allow While Using App", "Don’t Allow"},
	} {
		if d := (Dialog{Buttons: buttons}); d.IsTrackingPrompt() {
			t.Errorf("%q is taken for a tracking prompt", buttons)
		}
	}
}

// attInvisible are the declines whose Apple string holds a character that is easy to lose in a copy: a no-break space
// (U+00A0) in cs, pl, ru and sk, and a zero-width non-joiner (U+200C) in kn, or and te. The table keeps the exact
// characters, so these are checked byte for byte.
var attInvisible = []struct{ locale, decline string }{
	{"cs", "Požádat aplikaci o nesledování"},
	{"pl", "Poproś aplikację o nieśledzenie"},
	{"ru", "Попросить не отслеживать"},
	{"sk", "Požiadať apku o nesledovanie"},
	{"kn", "ಆ್ಯಪ್‌ಗೆ ಟ್ರ್ಯಾಕ್ ಮಾಡಬೇಡಿ ಎಂದು ಹೇಳಿ"},
	{"or", "ଟ୍ରାକ୍ ନକରିବା ପାଇଁ ଆପ୍‌କୁ କୁହନ୍ତୁ"},
	{"te", "ట్ర్యాక్ చేయకూడదని యాప్‌కి చెప్పండి"},
}

func TestATTDeclineKeepsItsInvisibleCharacters(t *testing.T) {
	for _, c := range attInvisible {
		t.Run(c.locale, func(t *testing.T) {
			if got := iosATTDecline[c.locale]; got != c.decline {
				t.Errorf("iosATTDecline[%s] = %q, want %q", c.locale, got, c.decline)
			}
			if !strings.ContainsAny(c.decline, " ‌") {
				t.Errorf("%q has no no-break space or ZWNJ, so the case tests nothing special", c.decline)
			}
			if !IsTrackingDecline(c.decline) {
				t.Errorf("IsTrackingDecline(%q) = false", c.decline)
			}
			// Folding turns a no-break space into a plain one, so the plain-space form is recognised too.
			if strings.Contains(c.decline, " ") && !IsTrackingDecline(strings.ReplaceAll(c.decline, " ", " ")) {
				t.Errorf("the plain-space form of %q is not recognised", c.decline)
			}
		})
	}
}

// TestTrackingDeclineIsNotAPermissionLabel: every label that IsTrackingDecline recognises must be the decline of its own
// locale's ATT alert, listed under deny. A permission alert's label that matched here would count as a tracking
// prompt. The same ATT string in two regional locales (en and en_GB, for example) is fine.
func TestTrackingDeclineIsNotAPermissionLabel(t *testing.T) {
	for loc, byRole := range iosSystemLabels {
		for role, labels := range byRole {
			for _, l := range labels {
				if !IsTrackingDecline(l) {
					continue
				}
				if role != "deny" || normalize(iosATTDecline[loc]) != normalize(l) {
					t.Errorf("%s %s %q is taken for a tracking decline, but %s's tracking decline is %q",
						loc, role, l, loc, iosATTDecline[loc])
				}
			}
		}
	}
}
