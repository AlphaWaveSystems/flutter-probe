package detox

import "regexp"

// Detox API patterns.
var (
	// element(by.id('x')).tap()
	elemByIDTap = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.tap\(\)`)
	// element(by.text('x')).tap()
	elemByTextTap = regexp.MustCompile(`element\(by\.text\(['"](.*?)['"]\)\)\.tap\(\)`)
	// element(by.label('x')).tap()
	elemByLabelTap = regexp.MustCompile(`element\(by\.label\(['"](.*?)['"]\)\)\.tap\(\)`)

	// element(by.id('x')).typeText('v')
	elemByIDType = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.typeText\(['"](.*?)['"]\)`)
	// element(by.text('x')).typeText('v')
	elemByTextType = regexp.MustCompile(`element\(by\.text\(['"](.*?)['"]\)\)\.typeText\(['"](.*?)['"]\)`)

	// element(by.id('x')).replaceText('v')
	elemByIDReplace = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.replaceText\(['"](.*?)['"]\)`)

	// element(by.id('x')).clearText()
	elemByIDClear = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.clearText\(\)`)

	// element(by.id('x')).longPress()
	elemByIDLongPress = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.longPress\(\)`)
	elemByTextLongPress = regexp.MustCompile(`element\(by\.text\(['"](.*?)['"]\)\)\.longPress\(\)`)

	// element(by.id('x')).swipe('direction')
	elemByIDSwipe = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.swipe\(['"](.*?)['"]\)`)

	// element(by.id('x')).scroll(N, 'direction')
	elemByIDScroll = regexp.MustCompile(`element\(by\.id\(['"](.*?)['"]\)\)\.scroll\(\d+,\s*['"](.*?)['"]\)`)

	// expect(element(by.text('x'))).toBeVisible()
	expectTextVisible = regexp.MustCompile(`expect\(element\(by\.text\(['"](.*?)['"]\)\)\)\.toBeVisible\(\)`)
	expectIDVisible = regexp.MustCompile(`expect\(element\(by\.id\(['"](.*?)['"]\)\)\)\.toBeVisible\(\)`)

	// expect(element(by.text('x'))).not.toBeVisible() / .not.toExist()
	expectTextNotVisible = regexp.MustCompile(`expect\(element\(by\.text\(['"](.*?)['"]\)\)\)\.not\.(?:toBeVisible|toExist)\(\)`)
	expectIDNotVisible = regexp.MustCompile(`expect\(element\(by\.id\(['"](.*?)['"]\)\)\)\.not\.(?:toBeVisible|toExist)\(\)`)

	// expect(element(by.id('x'))).toHaveText('v')
	expectIDHaveText = regexp.MustCompile(`expect\(element\(by\.id\(['"](.*?)['"]\)\)\)\.toHaveText\(['"](.*?)['"]\)`)

	// waitFor(element(by.text('x'))).toBeVisible()
	waitForTextVisible = regexp.MustCompile(`waitFor\(element\(by\.text\(['"](.*?)['"]\)\)\)\.toBeVisible\(\)`)
	waitForIDVisible = regexp.MustCompile(`waitFor\(element\(by\.id\(['"](.*?)['"]\)\)\)\.toBeVisible\(\)`)

	// device.launchApp()
	deviceLaunch = regexp.MustCompile(`device\.launchApp\(`)
	// device.reloadReactNative()
	deviceReload = regexp.MustCompile(`device\.reloadReactNative\(\)`)
	// device.takeScreenshot('name')
	deviceScreenshot = regexp.MustCompile(`device\.takeScreenshot\(['"](.*?)['"]\)`)

	// Block patterns.
	describeBlock = regexp.MustCompile(`describe\(['"](.*?)['"]`)
	itBlock       = regexp.MustCompile(`(?:^|\s)it\(['"](.*?)['"]`)
	beforeBlock   = regexp.MustCompile(`before(?:All|Each)\(`)
	afterBlock    = regexp.MustCompile(`after(?:All|Each)\(`)
)
