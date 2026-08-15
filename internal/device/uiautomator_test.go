package device

import (
	"image"
	"strings"
	"testing"
)

// sampleDump is a trimmed but structurally real uiautomator dump: a root
// container with two leaf nodes, one matched by text and one only
// identifiable by resource-id (the common case for icon-only buttons).
const sampleDump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" resource-id="" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
    <node index="0" text="Choose from Gallery" resource-id="com.android.documentsui:id/item_title" class="android.widget.TextView" bounds="[100,200][500,300]">
    </node>
    <node index="1" text="" resource-id="com.android.documentsui:id/icon_thumb" class="android.widget.ImageView" bounds="[600,200][700,300]">
    </node>
  </node>
</hierarchy>
`

func TestFindNativeElement_MatchesByText(t *testing.T) {
	rect, found, err := FindNativeElement(sampleDump, "gallery")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected to find an element matching \"gallery\"")
	}
	want := image.Rect(100, 200, 500, 300)
	if rect != want {
		t.Errorf("bounds: got %v, want %v", rect, want)
	}
}

func TestFindNativeElement_MatchesByResourceID(t *testing.T) {
	rect, found, err := FindNativeElement(sampleDump, "icon_thumb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected to find an element matching \"icon_thumb\"")
	}
	want := image.Rect(600, 200, 700, 300)
	if rect != want {
		t.Errorf("bounds: got %v, want %v", rect, want)
	}
}

func TestFindNativeElement_CaseInsensitive(t *testing.T) {
	_, found, err := FindNativeElement(sampleDump, "CHOOSE FROM GALLERY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected a case-insensitive match")
	}
}

func TestFindNativeElement_NoMatchReturnsFoundFalse(t *testing.T) {
	_, found, err := FindNativeElement(sampleDump, "does not exist anywhere")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for a query with no match")
	}
}

func TestFindNativeElement_InvalidXMLErrors(t *testing.T) {
	_, _, err := FindNativeElement("not xml at all {{{", "anything")
	if err == nil {
		t.Fatal("expected an error for malformed XML")
	}
}

func TestFindNativeElement_MalformedBoundsErrors(t *testing.T) {
	dump := strings.Replace(sampleDump, `bounds="[100,200][500,300]"`, `bounds="garbage"`, 1)
	_, _, err := FindNativeElement(dump, "gallery")
	if err == nil {
		t.Fatal("expected an error for a malformed bounds attribute")
	}
}
