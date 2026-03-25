package runner

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestRandomEmail(t *testing.T) {
	email := randomEmail()
	if !strings.Contains(email, "@") {
		t.Errorf("email missing @: %q", email)
	}
	if !strings.HasSuffix(email, "@test.probe") {
		t.Errorf("email wrong domain: %q", email)
	}
}

func TestRandomName(t *testing.T) {
	name := randomName()
	if name == "" {
		t.Error("name is empty")
	}
	parts := strings.Fields(name)
	if len(parts) != 2 {
		t.Errorf("name should have first+last: %q", name)
	}
}

func TestRandomPhone(t *testing.T) {
	phone := randomPhone()
	if !strings.HasPrefix(phone, "+1-555-") {
		t.Errorf("phone wrong prefix: %q", phone)
	}
}

func TestRandomUUID(t *testing.T) {
	uuid := randomUUID()
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(uuid) {
		t.Errorf("invalid UUID v4: %q", uuid)
	}
}

func TestRandomNumber(t *testing.T) {
	for i := 0; i < 20; i++ {
		result := randomNumber("10,20")
		n, err := strconv.Atoi(result)
		if err != nil {
			t.Fatalf("not a number: %q", result)
		}
		if n < 10 || n > 20 {
			t.Errorf("out of range [10,20]: %d", n)
		}
	}
}

func TestRandomNumber_SameMinMax(t *testing.T) {
	result := randomNumber("5,5")
	if result != "5" {
		t.Errorf("expected 5, got %q", result)
	}
}

func TestRandomNumber_Default(t *testing.T) {
	result := randomNumber("")
	n, err := strconv.Atoi(result)
	if err != nil {
		t.Fatalf("not a number: %q", result)
	}
	if n < 1 || n > 100 {
		t.Errorf("default range [1,100]: got %d", n)
	}
}

func TestRandomText(t *testing.T) {
	text := randomText("15")
	if len(text) != 15 {
		t.Errorf("expected length 15, got %d: %q", len(text), text)
	}
}

func TestRandomText_Default(t *testing.T) {
	text := randomText("")
	if len(text) != 10 {
		t.Errorf("default length 10, got %d", len(text))
	}
}

func TestResolveRandomVars(t *testing.T) {
	s := `email: <random.email>, name: <random.name>`
	result := resolveRandomVars(s)

	if strings.Contains(result, "<random.") {
		t.Errorf("unresolved random vars: %q", result)
	}
	if !strings.Contains(result, "@test.probe") {
		t.Errorf("email not resolved: %q", result)
	}
}

func TestResolveRandomVars_WithArgs(t *testing.T) {
	s := `num: <random.number(5,10)>, text: <random.text(3)>`
	result := resolveRandomVars(s)

	if strings.Contains(result, "<random.") {
		t.Errorf("unresolved: %q", result)
	}
}

func TestResolveRandomVars_NoMatch(t *testing.T) {
	s := "hello world <username>"
	result := resolveRandomVars(s)
	if result != s {
		t.Errorf("should not modify non-random vars: %q", result)
	}
}

func TestResolveRandomVars_Unknown(t *testing.T) {
	s := "<random.unknown>"
	result := resolveRandomVars(s)
	if result != s {
		t.Errorf("unknown random type should remain: %q", result)
	}
}
