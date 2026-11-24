package app

import "testing"

// Enter inserts a trailing newline that splitLines drops; the buffer must
// keep it so the next line can be typed.
func TestLineTextKeepsTypedNewlines(t *testing.T) {
	a := &App{}
	list := []string{`D:\icons`}
	text := a.lineText("dirs", list)
	if *text != `D:\icons` {
		t.Fatalf("initial text = %q", *text)
	}
	*text += "\n"
	list = splitLines(*text)
	if got := *a.lineText("dirs", list); got != "D:\\icons\n" {
		t.Fatalf("trailing newline lost: %q", got)
	}
	*text += `D:\tools`
	list = splitLines(*text)
	if len(list) != 2 || list[1] != `D:\tools` {
		t.Fatalf("list = %q", list)
	}
	// An external change (reset, reopen) rebuilds the buffer.
	if got := *a.lineText("dirs", []string{`C:\x`}); got != `C:\x` {
		t.Fatalf("external change not synced: %q", got)
	}
}
