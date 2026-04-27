package parser

// TokenType represents a lexical token type in ProbeScript.
type TokenType int

const (
	// Literals
	TOKEN_EOF TokenType = iota
	TOKEN_NEWLINE
	TOKEN_INDENT
	TOKEN_DEDENT
	TOKEN_STRING  // "quoted text"
	TOKEN_INT     // 3
	TOKEN_FLOAT   // 3.14
	TOKEN_IDENT   // unquoted word
	TOKEN_ID      // #login_button
	TOKEN_ORDINAL // 1st 2nd 3rd etc.

	// Top-level keywords
	TOKEN_TEST
	TOKEN_RECIPE
	TOKEN_USE
	TOKEN_BEFORE
	TOKEN_AFTER
	TOKEN_ON

	// Action verbs (anchors)
	TOKEN_OPEN
	TOKEN_TAP
	TOKEN_TYPE
	TOKEN_SEE
	TOKEN_DONT_SEE // "don't see"
	TOKEN_WAIT
	TOKEN_SWIPE
	TOKEN_SCROLL
	TOKEN_GO_BACK    // "go back"
	TOKEN_LONG_PRESS // "long press"
	TOKEN_DOUBLE_TAP // "double tap"
	TOKEN_CLEAR
	TOKEN_CLOSE
	TOKEN_DRAG
	TOKEN_PINCH
	TOKEN_ROTATE
	TOKEN_TOGGLE
	TOKEN_SHAKE
	TOKEN_PRESS

	// Control flow
	TOKEN_IF
	TOKEN_OTHERWISE
	TOKEN_REPEAT
	TOKEN_TIMES
	TOKEN_FOR
	TOKEN_EACH
	TOKEN_WHEN
	TOKEN_RESPOND

	// Lifecycle
	TOKEN_EACH_KW   // used as "each" in "before each test"
	TOKEN_FAILURE   // "failure"
	TOKEN_LIFECYCLE // before/after each test / on failure

	// Qualifiers / filler
	TOKEN_THE
	TOKEN_A
	TOKEN_AN
	TOKEN_ON_KW // filler "on" (same semantic as TOKEN_ON above, alias)
	TOKEN_IN
	TOKEN_INTO
	TOKEN_AT
	TOKEN_OF
	TOKEN_TO
	TOKEN_FROM
	TOKEN_FOR_KW
	TOKEN_IS
	TOKEN_ARE
	TOKEN_THAT
	TOKEN_THIS
	TOKEN_IT
	TOKEN_WITH
	TOKEN_AS
	TOKEN_AND
	TOKEN_UNTIL
	TOKEN_APPEARS
	TOKEN_DISAPPEARS
	TOKEN_ENABLED
	TOKEN_DISABLED
	TOKEN_CHECKED
	TOKEN_CONTAINS
	TOKEN_EXACTLY
	TOKEN_BUTTON
	TOKEN_FIELD
	TOKEN_APP
	TOKEN_PAGE
	TOKEN_NETWORK
	TOKEN_IDLE
	TOKEN_LOAD
	TOKEN_SECONDS
	TOKEN_SECOND

	// Extras
	TOKEN_COLON  // :
	TOKEN_HASH   // inline comment #
	TOKEN_DART   // "dart" after run
	TOKEN_RUN
	TOKEN_GET
	TOKEN_POST
	TOKEN_PUT
	TOKEN_DELETE
	TOKEN_BODY
	TOKEN_EXAMPLES // "examples"
	TOKEN_MATCHING // "matching"
	TOKEN_BETWEEN  // "between"
	TOKEN_TAKE
	TOKEN_COMPARE
	TOKEN_SCREENSHOT
	TOKEN_CALLED
	TOKEN_DUMP
	TOKEN_WIDGET
	TOKEN_TREE
	TOKEN_SAVE
	TOKEN_DEVICE
	TOKEN_LOGS
	TOKEN_PAUSE
	TOKEN_LOG
	TOKEN_BYPASS // custom recipe call placeholder
	TOKEN_BACK
	TOKEN_HOME
	TOKEN_RESTART    // "restart the app"
	TOKEN_CLEAR_DATA // "clear app data" (compound)

	// Permission commands
	TOKEN_ALLOW      // "allow"
	TOKEN_DENY       // "deny"
	TOKEN_GRANT      // "grant"
	TOKEN_REVOKE     // "revoke"
	TOKEN_PERMISSION // "permission"
	TOKEN_ALL        // "all"

	// New E2E commands (v0.4.0)
	TOKEN_KILL            // "kill"
	TOKEN_COPY            // "copy"
	TOKEN_PASTE           // "paste"
	TOKEN_SET_LOCATION    // compound: "set location"
	TOKEN_VERIFY_BROWSER  // compound: "verify external browser"
	TOKEN_CALL            // "call"

	// Relational selectors (v0.5.7)
	TOKEN_BELOW  // "below"
	TOKEN_ABOVE  // "above"
	TOKEN_LEFT   // "left"
	TOKEN_RIGHT  // "right"

	// New keywords (v0.5.7)
	TOKEN_FOCUSED    // "focused" — state check in see
	TOKEN_LINK       // "link" — open link "url"
	TOKEN_ANIMATIONS // "animations" — wait for animations to end
	TOKEN_STORE      // "store" — store "value" as varName
)

// Token is a single lexical unit.
type Token struct {
	Type    TokenType
	Literal string // raw text
	Line    int
	Col     int
}

func (t Token) String() string {
	return t.Literal
}

// keywords maps lowercase words to token types.
var keywords = map[string]TokenType{
	"test":         TOKEN_TEST,
	"recipe":       TOKEN_RECIPE,
	"use":          TOKEN_USE,
	"before":       TOKEN_BEFORE,
	"after":        TOKEN_AFTER,
	"on":           TOKEN_ON_KW,
	"open":         TOKEN_OPEN,
	"tap":          TOKEN_TAP,
	"type":         TOKEN_TYPE,
	"see":          TOKEN_SEE,
	"wait":         TOKEN_WAIT,
	"swipe":        TOKEN_SWIPE,
	"scroll":       TOKEN_SCROLL,
	"clear":        TOKEN_CLEAR,
	"close":        TOKEN_CLOSE,
	"drag":         TOKEN_DRAG,
	"pinch":        TOKEN_PINCH,
	"rotate":       TOKEN_ROTATE,
	"toggle":       TOKEN_TOGGLE,
	"shake":        TOKEN_SHAKE,
	"press":        TOKEN_PRESS,
	"if":           TOKEN_IF,
	"otherwise":    TOKEN_OTHERWISE,
	"repeat":       TOKEN_REPEAT,
	"times":        TOKEN_TIMES,
	"for":          TOKEN_FOR_KW,
	"each":         TOKEN_EACH_KW,
	"when":         TOKEN_WHEN,
	"respond":      TOKEN_RESPOND,
	"the":          TOKEN_THE,
	"a":            TOKEN_A,
	"an":           TOKEN_AN,
	"in":           TOKEN_IN,
	"into":         TOKEN_INTO,
	"at":           TOKEN_AT,
	"of":           TOKEN_OF,
	"to":           TOKEN_TO,
	"from":         TOKEN_FROM,
	"is":           TOKEN_IS,
	"are":          TOKEN_ARE,
	"that":         TOKEN_THAT,
	"this":         TOKEN_THIS,
	"it":           TOKEN_IT,
	"with":         TOKEN_WITH,
	"as":           TOKEN_AS,
	"and":          TOKEN_AND,
	"until":        TOKEN_UNTIL,
	"appears":      TOKEN_APPEARS,
	"disappears":   TOKEN_DISAPPEARS,
	"enabled":      TOKEN_ENABLED,
	"disabled":     TOKEN_DISABLED,
	"checked":      TOKEN_CHECKED,
	"contains":     TOKEN_CONTAINS,
	"exactly":      TOKEN_EXACTLY,
	"button":       TOKEN_BUTTON,
	"field":        TOKEN_FIELD,
	"app":          TOKEN_APP,
	"page":         TOKEN_PAGE,
	"network":      TOKEN_NETWORK,
	"idle":         TOKEN_IDLE,
	"load":         TOKEN_LOAD,
	"seconds":      TOKEN_SECONDS,
	"second":       TOKEN_SECOND,
	"dart":         TOKEN_DART,
	"run":          TOKEN_RUN,
	"get":          TOKEN_GET,
	"post":         TOKEN_POST,
	"put":          TOKEN_PUT,
	"delete":       TOKEN_DELETE,
	"body":         TOKEN_BODY,
	"examples":     TOKEN_EXAMPLES,
	"matching":     TOKEN_MATCHING,
	"between":      TOKEN_BETWEEN,
	"take":         TOKEN_TAKE,
	"compare":      TOKEN_COMPARE,
	"screenshot":   TOKEN_SCREENSHOT,
	"called":       TOKEN_CALLED,
	"dump":         TOKEN_DUMP,
	"widget":       TOKEN_WIDGET,
	"tree":         TOKEN_TREE,
	"save":         TOKEN_SAVE,
	"device":       TOKEN_DEVICE,
	"logs":         TOKEN_LOGS,
	"pause":        TOKEN_PAUSE,
	"log":          TOKEN_LOG,
	"back":         TOKEN_BACK,
	"home":         TOKEN_HOME,
	"failure":      TOKEN_FAILURE,
	"restart":      TOKEN_RESTART,
	"allow":        TOKEN_ALLOW,
	"deny":         TOKEN_DENY,
	"grant":        TOKEN_GRANT,
	"revoke":       TOKEN_REVOKE,
	"permission":   TOKEN_PERMISSION,
	"permissions":  TOKEN_PERMISSION,
	"all":          TOKEN_ALL,
	"kill":         TOKEN_KILL,
	"copy":         TOKEN_COPY,
	"paste":        TOKEN_PASTE,
	"call":         TOKEN_CALL,
	"verify":       TOKEN_IDENT,
	"set":          TOKEN_IDENT,
	"location":     TOKEN_IDENT,
	"clipboard":    TOKEN_IDENT,

	// Relational / new keywords (v0.5.7)
	"below":      TOKEN_BELOW,
	"above":      TOKEN_ABOVE,
	"left":       TOKEN_LEFT,
	"right":      TOKEN_RIGHT,
	"focused":    TOKEN_FOCUSED,
	"link":       TOKEN_LINK,
	"animations": TOKEN_ANIMATIONS,
	"animation":  TOKEN_ANIMATIONS,
	"store":      TOKEN_STORE,
}

// fillerWords are stripped by the forgiving parser.
var fillerWords = map[TokenType]bool{
	TOKEN_THE:    true,
	TOKEN_A:      true,
	TOKEN_AN:     true,
	TOKEN_ON_KW:  true,
	TOKEN_IN:     true,
	TOKEN_INTO:   true,
	TOKEN_AT:     true,
	TOKEN_OF:     true,
	TOKEN_FROM:   true,
	TOKEN_IS:     true,
	TOKEN_ARE:    true,
	TOKEN_THAT:   true,
	TOKEN_THIS:   true,
	TOKEN_IT:     true,
	TOKEN_FOR_KW: true,
}

// IsFiller returns true if the token is a filler word.
func IsFiller(t TokenType) bool {
	return fillerWords[t]
}
