package parser

// ---- Selector types ----

type SelectorKind int

const (
	SelectorText     SelectorKind = iota // "visible text"
	SelectorID                           // #key_name
	SelectorType                         // widget type name
	SelectorOrdinal                      // 1st "text", 3rd item in "List"
	SelectorPositional                   // "text" in "container"
)

// Selector describes how to locate a widget.
type Selector struct {
	Kind      SelectorKind
	Text      string // text / id / type name
	Ordinal   int    // used with SelectorOrdinal
	Container string // used with SelectorPositional
}

// ---- Target for wait/assertion state checks ----

type StateCheck int

const (
	StateNone     StateCheck = iota
	StateEnabled
	StateDisabled
	StateChecked
	StateContains // contains "text"
)

// ---- Node interface ----

// Node is implemented by all AST nodes.
type Node interface {
	nodeType() string
	GetLine() int
}

// ---- Program ----

type Program struct {
	Uses    []UseStmt
	Recipes []RecipeDef
	Hooks   []HookDef
	Tests   []TestDef
}

// ---- UseStmt ----

type UseStmt struct {
	Path string
	Line int
}

func (u UseStmt) nodeType() string { return "use" }
func (u UseStmt) GetLine() int     { return u.Line }

// ---- RecipeDef ----

type RecipeDef struct {
	Name   string
	Params []string
	Body   []Step
	Line   int
}

func (r RecipeDef) nodeType() string { return "recipe" }
func (r RecipeDef) GetLine() int     { return r.Line }

// ---- HookDef ----

type HookKind string

const (
	HookBeforeEach HookKind = "before_each"
	HookAfterEach  HookKind = "after_each"
	HookOnFailure  HookKind = "on_failure"
	HookBeforeAll  HookKind = "before_all"
	HookAfterAll   HookKind = "after_all"
)

type HookDef struct {
	Kind HookKind
	Body []Step
	Line int
}

func (h HookDef) nodeType() string { return "hook" }
func (h HookDef) GetLine() int     { return h.Line }

// ---- TestDef ----

type TestDef struct {
	Name     string
	Tags     []string
	Body     []Step
	Examples *ExamplesBlock
	Line     int
}

func (t TestDef) nodeType() string { return "test" }
func (t TestDef) GetLine() int     { return t.Line }

// ---- ExamplesBlock ----

type ExamplesBlock struct {
	Headers []string
	Rows    [][]string
	Source  string // CSV file path (empty means inline data)
	Line    int
}

// ---- Step interface ----

type Step interface {
	Node
	stepType() string
}

// ---- ActionStep ---- (open, tap, type, swipe, scroll, go back, etc.)

type ActionVerb string

const (
	VerbOpen       ActionVerb = "open"
	VerbTap        ActionVerb = "tap"
	VerbType       ActionVerb = "type"
	VerbSwipe      ActionVerb = "swipe"
	VerbScroll     ActionVerb = "scroll"
	VerbGoBack     ActionVerb = "go_back"
	VerbLongPress  ActionVerb = "long_press"
	VerbDoubleTap  ActionVerb = "double_tap"
	VerbClear      ActionVerb = "clear"
	VerbClose      ActionVerb = "close"
	VerbDrag       ActionVerb = "drag"
	VerbPinch      ActionVerb = "pinch"
	VerbRotate     ActionVerb = "rotate"
	VerbToggle     ActionVerb = "toggle"
	VerbShake      ActionVerb = "shake"
	VerbPress      ActionVerb = "press"
	VerbTakeShot    ActionVerb = "take_screenshot"
	VerbCompareShot ActionVerb = "compare_screenshot"
	VerbDumpTree   ActionVerb = "dump_widget_tree"
	VerbSaveLogs   ActionVerb = "save_logs"
	VerbPause        ActionVerb = "pause"
	VerbLog          ActionVerb = "log"
	VerbRestart         ActionVerb = "restart"
	VerbClearAppData    ActionVerb = "clear_app_data"
	VerbAllowPermission ActionVerb = "allow_permission"
	VerbDenyPermission  ActionVerb = "deny_permission"
	VerbGrantAllPerms   ActionVerb = "grant_all_permissions"
	VerbRevokeAllPerms  ActionVerb = "revoke_all_permissions"
	VerbKill            ActionVerb = "kill"
	VerbCopyClipboard   ActionVerb = "copy_clipboard"
	VerbPasteClipboard  ActionVerb = "paste_clipboard"
	VerbSetLocation     ActionVerb = "set_location"
	VerbVerifyBrowser   ActionVerb = "verify_browser"
)

type SwipeDirection string

const (
	SwipeUp    SwipeDirection = "up"
	SwipeDown  SwipeDirection = "down"
	SwipeLeft  SwipeDirection = "left"
	SwipeRight SwipeDirection = "right"
)

type ActionStep struct {
	Verb      ActionVerb
	Sel       *Selector   // target selector (may be nil for directionless actions)
	Text      string      // for "type" — the text to enter
	Direction SwipeDirection // for swipe / scroll
	Name      string      // for screenshot name, rotate direction, locale, etc.
	To        *Selector   // for drag: destination
	Line      int
}

func (a ActionStep) nodeType() string { return "action" }
func (a ActionStep) GetLine() int     { return a.Line }
func (a ActionStep) stepType() string { return "action" }

// ---- AssertStep ----

type AssertStep struct {
	Negated  bool     // don't see
	Sel      Selector
	Count    int        // see exactly N
	Check    StateCheck // is enabled, contains, etc.
	CheckVal string     // for contains
	Pattern  string     // regex for "matching"
	Line     int
}

func (a AssertStep) nodeType() string { return "assert" }
func (a AssertStep) GetLine() int     { return a.Line }
func (a AssertStep) stepType() string { return "assert" }

// ---- WaitStep ----

type WaitKind int

const (
	WaitDuration   WaitKind = iota // wait N seconds
	WaitAppears                    // wait until "X" appears
	WaitDisappears                 // wait until "X" disappears
	WaitPageLoad                   // wait for page to load
	WaitNetworkIdle                // wait until network is idle
	WaitSelector                   // wait until #id disappears/appears
)

type WaitStep struct {
	Kind     WaitKind
	Target   string   // text or selector
	Duration float64  // seconds
	Line     int
}

func (w WaitStep) nodeType() string { return "wait" }
func (w WaitStep) GetLine() int     { return w.Line }
func (w WaitStep) stepType() string { return "wait" }

// ---- ConditionalStep ----

type ConditionalStep struct {
	Condition string // text that may appear
	Then      []Step
	Else      []Step // otherwise branch
	Line      int
}

func (c ConditionalStep) nodeType() string { return "conditional" }
func (c ConditionalStep) GetLine() int     { return c.Line }
func (c ConditionalStep) stepType() string { return "conditional" }

// ---- LoopStep ----

type LoopStep struct {
	Count int
	Body  []Step
	Line  int
}

func (l LoopStep) nodeType() string { return "loop" }
func (l LoopStep) GetLine() int     { return l.Line }
func (l LoopStep) stepType() string { return "loop" }

// ---- DartBlock ----

type DartBlock struct {
	Code string
	Line int
}

func (d DartBlock) nodeType() string { return "dart" }
func (d DartBlock) GetLine() int     { return d.Line }
func (d DartBlock) stepType() string { return "dart" }

// ---- MockBlock ----

type MockBlock struct {
	Method   string // GET POST PUT DELETE
	Path     string // /api/products
	Status   int
	Body     string // JSON string
	Line     int
}

func (m MockBlock) nodeType() string { return "mock" }
func (m MockBlock) GetLine() int     { return m.Line }
func (m MockBlock) stepType() string { return "mock" }

// ---- RecipeCall ----

type RecipeCall struct {
	Name   string
	Args   []string
	Line   int
}

func (r RecipeCall) nodeType() string { return "recipe_call" }
func (r RecipeCall) GetLine() int     { return r.Line }
func (r RecipeCall) stepType() string { return "recipe_call" }

// ---- HTTPCallStep ----

type HTTPCallStep struct {
	Method string // GET, POST, PUT, DELETE
	URL    string
	Body   string // optional request body
	Line   int
}

func (h HTTPCallStep) nodeType() string { return "http_call" }
func (h HTTPCallStep) GetLine() int     { return h.Line }
func (h HTTPCallStep) stepType() string { return "http_call" }
