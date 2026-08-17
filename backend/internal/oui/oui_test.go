package oui

import "testing"

func TestLookupKnown(t *testing.T) {
	if v := Lookup("3c:d9:2b:12:34:56"); v != "Hewlett Packard" {
		t.Fatalf("HP lookup got %q", v)
	}
	if v := Lookup("FC-9F-FD-9F-32-97"); v == "" {
		t.Fatal("Hikvision lookup empty")
	}
	if v := Lookup(""); v != "" {
		t.Fatalf("empty mac got %q", v)
	}
}

func TestInferType(t *testing.T) {
	if typ := InferType("3c:d9:2b:12:34:56"); typ != "电脑" {
		t.Fatalf("HP type got %q", typ)
	}
	if typ := InferType("fc:9f:fd:9f:32:97"); typ != "摄像头" {
		t.Fatalf("Hikvision type got %q", typ)
	}
	if typ := InferType("zz:zz"); typ != "" {
		t.Fatalf("invalid mac type got %q", typ)
	}
}
