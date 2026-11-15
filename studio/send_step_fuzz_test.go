package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// A property test for Save as step. For any typed request, whatever the code decides, these hold:
//
//   - it does not panic;
//   - a saved line is a single line that the real ProbeScript parser reads back as a call step with the method that was typed;
//   - a request that holds a ${NAME} or a <name> the runner would fill in is never saved;
//   - a secret with a recognisable shape (a token with letters and digits, a card number, an email address, a JWT, a hex or base64
//     string) planted in any position (query value, query under a credential name, path segment, JSON value, JSON array, form value,
//     free text, after a credential name, an XML element, an Authorization line) is never in what is saved, nor in an error.
//
// A plain `go test` runs the seed corpus below, which holds the classes the reviews found; `go test -fuzz=FuzzSaveStep` explores more.
// The fuzz run is not part of CI.

var plantedSecrets = []string{
	"Zk9qLm2Xp7Rt",
	"4111111111111111",
	"jane.doe@example.com",
	"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV",
	"9f86d081884c7d659a2feaa0c55ad015",
	"dGVzdF9zZWNyZXRfMTIzNDU2Nzg5MA==",
}

// plant puts the secret into a request at one position, with the noise around it.
func plant(position uint8, secret, noise string) (method, url, body string) {
	word := regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(noise, "")
	if word == "" {
		word = "k"
	}
	if len(word) > 20 {
		word = word[:20]
	}
	base := "https://api.example.test/v1/items"
	switch position % 12 {
	case 0:
		return "GET", base + "?x=" + secret, ""
	case 1:
		return "GET", base + "?token=" + secret + "&id=7", ""
	case 2:
		return "GET", base + "/" + secret + "/details", ""
	case 3:
		return "POST", base, `{"` + word + `":"` + secret + `","n":1}`
	case 4:
		return "POST", base, `{"password":"` + secret + `"}`
	case 5:
		return "PUT", base, `{"list":["a","` + secret + `",{"deep":["` + secret + `"]}]}`
	case 6:
		return "POST", base, "a=1&" + word + "=" + secret
	case 7:
		return "POST", base, noise + " " + secret + " " + noise
	case 8:
		return "DELETE", base, "note\npassword: " + secret + "\nafter"
	case 9:
		return "POST", base, `<login><password type="plain">` + secret + `</password></login>`
	case 10:
		return "POST", base, "Authorization: Bearer " + secret
	default:
		return "POST", base, `{"` + secret + `":true}` // a secret as a key
	}
}

func FuzzSaveStep(f *testing.F) {
	// The planted secrets in every position, with and without noise.
	for s := range plantedSecrets {
		for p := 0; p < 12; p++ {
			f.Add("POST", "https://api.example.test/x", "", uint8(s), uint8(p), "")
			f.Add("POST", "https://api.example.test/x", "", uint8(s), uint8(p), "some words, 12 and \"quotes\" \\ back")
		}
	}
	// The classes the reviews found, as typed requests.
	for _, c := range [][3]string{
		{"POST", "https://api.example.test/x", `{"note":"${REV_DEPLOY_TOKEN}"}`},
		{"GET", "https://api.example.test/x?${REV_DEPLOY_TOKEN}=1", ""},
		{"GET", "https://api.example.test/${HOME}/x", ""},
		{"POST", "https://api.example.test/x", `{"email":"<random.email>"}`},
		{"POST", "https://api.example.test/x", "<response.body> and <clipboard> and <name>"},
		{"POST", "https://api.example.test/x", "$5 and ${1} and < a > and %24%7BHOME%7D"},
		{"POST", "https://api.example.test/x", "a­b  ‮"},
		{"POST", "https://api.example.test/x", "line\r\nbreak and\u0085more"},
		{"POST", "https://api.example.test/x", "a\x00b\x07"},
		{"PATCH", "https://api.example.test/x", "{}"},
		{"GET", "https://user:pass@api.example.test/x", ""},
		{"GET", "file:///etc/passwd", ""},
		{"GET", "https://api.example.test/x?a%0Ab=1&c%E2%80%A8d=2", ""},
		{"POST", "https://api.example.test/x?user=bob&path=/a&home=x", `{"user":"bob"}`},
		{"POST", "https://api.example.test/x", `<a href="x">y</a><password type="p">hunter two</password>`},
		{"POST", "https://api.example.test/x", `{"k":"` + strings.Repeat("a", 5000) + `"}`},
		{"POST", "https://api.example.test/x", strings.Repeat("{", 2000)},
		{"hunter2-Xk9pq", "https://api.example.test/x", ""},
	} {
		f.Add(c[0], c[1], c[2], uint8(255), uint8(0), "")
	}

	f.Fuzz(func(t *testing.T, method, rawURL, body string, secretKind, position uint8, noise string) {
		in := SaveStepInput{Method: method, URL: rawURL, Body: body}
		var secret string
		if int(secretKind) < len(plantedSecrets) {
			secret = plantedSecrets[secretKind]
			if position%12 == 11 && secretKind == 0 {
				// A plain token (letters and digits, no strong shape) written as an object key is a documented limit: an ordinary key such
				// as "address1_line" looks the same, so a key is only refused when it is a hex string, a JWT or padded base64, and an
				// email or a card number is replaced.
				position = 3
			}
			in.Method, in.URL, in.Body = plant(position, secret, noise)
		}
		step, err := buildCallStep(in) // must not panic
		if err != nil {
			if secret != "" && strings.Contains(strings.ToLower(err.Error()), strings.ToLower(secret)) {
				t.Fatalf("the error holds the planted secret: %v", err)
			}
			return
		}
		line := step.Lines[len(step.Lines)-1]
		for _, l := range step.Lines {
			if strings.ContainsAny(l, "\n\r  \u0085") {
				t.Fatalf("a saved line is not a single line: %q", l)
			}
		}
		// It reads back as a call with the method that was typed.
		prog, perr := parser.ParseFile("test \"x\"\n  " + strings.Join(step.Lines, "\n  ") + "\n")
		if perr != nil || len(prog.Tests) != 1 {
			t.Fatalf("the saved step does not parse: %v\n%s", perr, strings.Join(step.Lines, "\n"))
		}
		var call *parser.HTTPCallStep
		for _, s := range prog.Tests[0].Body {
			if c, ok := s.(parser.HTTPCallStep); ok {
				call = &c
			}
		}
		if call == nil || call.Method != strings.ToUpper(strings.TrimSpace(in.Method)) {
			t.Fatalf("the saved step is not a %q call: %+v", in.Method, call)
		}
		// What the runner would fill in is never saved as typed: the only ${...} are the generated ones, and no <name> is left.
		if envForm.MatchString(in.URL) || envForm.MatchString(in.Body) || varForm.MatchString(in.URL) || varForm.MatchString(in.Body) {
			t.Fatalf("a request with an interpolation form was saved: %q %q", in.URL, in.Body)
		}
		for _, m := range envForm.FindAllString(call.URL+"\x00"+call.Body, -1) {
			if !strings.HasPrefix(m, "${"+savedPrefix) {
				t.Fatalf("a ${...} that is not a generated one: %s", m)
			}
		}
		// The planted secret is not in the step, whatever its case, and neither is its core (without padding).
		if secret != "" {
			all := strings.ToLower(strings.Join(step.Lines, "\n") + "\n" + call.URL + "\n" + call.Body)
			for _, s := range []string{secret, strings.TrimRight(secret, "=")} {
				if strings.Contains(all, strings.ToLower(s)) {
					t.Fatalf("the planted secret %q is in the step (position %d):\n%s", s, position%12, strings.Join(step.Lines, "\n"))
				}
			}
		}
		_ = fmt.Sprint(line)
	})
}
