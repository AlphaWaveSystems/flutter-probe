package runner

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// randomVarPattern matches <random.TYPE> or <random.TYPE(args)> placeholders.
var randomVarPattern = regexp.MustCompile(`<random\.(\w+)(?:\(([^)]*)\))?>`)

// resolveRandomVars replaces <random.*> placeholders with generated values.
func resolveRandomVars(s string) string {
	return randomVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		m := randomVarPattern.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		kind := m[1]
		args := ""
		if len(m) > 2 {
			args = m[2]
		}

		switch kind {
		case "email":
			return randomEmail()
		case "name":
			return randomName()
		case "phone":
			return randomPhone()
		case "uuid":
			return randomUUID()
		case "number":
			return randomNumber(args)
		case "text":
			return randomText(args)
		default:
			return match
		}
	})
}

func randomEmail() string {
	id := randomAlphaNum(8)
	return id + "@test.probe"
}

var firstNames = []string{
	"Alice", "Bob", "Carlos", "Diana", "Erik",
	"Fatima", "George", "Hannah", "Ivan", "Julia",
	"Kenji", "Luna", "Miguel", "Nora", "Oscar",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones",
	"Garcia", "Martinez", "Anderson", "Taylor", "Thomas",
	"Lee", "Kim", "Patel", "Singh", "Muller",
}

func randomName() string {
	first := firstNames[randInt(len(firstNames))]
	last := lastNames[randInt(len(lastNames))]
	return first + " " + last
}

func randomPhone() string {
	return fmt.Sprintf("+1-555-%03d-%04d", randInt(1000), randInt(10000))
}

func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func randomNumber(args string) string {
	min, max := 1, 100
	if args != "" {
		parts := strings.SplitN(args, ",", 2)
		if len(parts) == 2 {
			if v, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				min = v
			}
			if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				max = v
			}
		}
	}
	if max <= min {
		return strconv.Itoa(min)
	}
	return strconv.Itoa(min + randInt(max-min+1))
}

func randomText(args string) string {
	length := 10
	if args != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(args)); err == nil && v > 0 {
			length = v
		}
	}
	return randomAlphaNum(length)
}

const alphaNum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomAlphaNum(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphaNum[randInt(len(alphaNum))]
	}
	return string(b)
}

func randInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}
