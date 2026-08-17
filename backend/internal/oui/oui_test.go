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

func TestIsRandom(t *testing.T) {
	cases := map[string]bool{
		"3c:d9:2b:12:34:56": false, // HP（全局唯一）
		"02:1a:2b:3c:4d:5e": true,  // LAA 位置位
		"4e:8f:aa:bb:cc:dd": true,  // iOS 私有地址常见段
		"32:ab:cd:ef:01:23": true,
		"30:ab:cd:ef:01:23": false, // 0x30 LAA 未置位
		"": false, "zz:zz": false,
	}
	for mac, want := range cases {
		if got := IsRandom(mac); got != want {
			t.Fatalf("IsRandom(%q)=%v, want %v", mac, got, want)
		}
	}
}
