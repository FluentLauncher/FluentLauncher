package main

import (
	"strings"
	"testing"
)

func TestHash(t *testing.T) {
	outputs := run("hash", "abc")
	if len(outputs) != 3 || !strings.Contains(outputs[0].Text, "ba7816bf8f01cfea") {
		t.Fatalf("hash = %+v", outputs)
	}
}

func TestJSON(t *testing.T) {
	outputs := run("json", `{"a":1,}`)
	if len(outputs) != 2 || !strings.Contains(outputs[0].Text, `"a": 1`) {
		t.Fatalf("json = %+v", outputs)
	}
}

func TestBase64(t *testing.T) {
	outputs := run("base64", "aGVsbG8=")
	if len(outputs) != 2 || outputs[0].Text != "hello" {
		t.Fatalf("base64 = %+v", outputs)
	}
}

func TestURL(t *testing.T) {
	outputs := run("urlencode", "a b")
	if len(outputs) == 0 || outputs[0].Text != "a%20b" {
		t.Fatalf("url = %+v", outputs)
	}
}

func TestTimeAndUUID(t *testing.T) {
	if outputs := run("time", ""); len(outputs) != 4 {
		t.Fatalf("time = %+v", outputs)
	}
	outputs := run("uuid", "2")
	if len(outputs) != 1 || len(strings.Split(outputs[0].Text, "\r\n")) != 2 {
		t.Fatalf("uuid = %+v", outputs)
	}
}
