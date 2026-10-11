package cmd

import (
	"strings"
	"testing"
)

func TestRejectSystemLabel(t *testing.T) {
	for _, key := range []string{
		"io.oci-sync.encrypted",
		"io.oci-sync.version",
		"io.oci-sync.",
		"io.oci-sync.anything",
	} {
		if err := rejectSystemLabel(key); err == nil {
			t.Errorf("rejectSystemLabel(%q) = nil, want error", key)
		}
	}

	for _, key := range []string{"app", "io.example.encrypted", "version", "my.io.oci-sync.x"} {
		if err := rejectSystemLabel(key); err != nil {
			t.Errorf("rejectSystemLabel(%q) = %v, want nil", key, err)
		}
	}
}

func TestParseLabels(t *testing.T) {
	got, err := parseLabels([]string{"app=myapp", "env=prod", "empty="}, true)
	if err != nil {
		t.Fatalf("parseLabels: %v", err)
	}
	want := map[string]string{"app": "myapp", "env": "prod", "empty": ""}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("label %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseLabelsValueMayContainEquals(t *testing.T) {
	got, err := parseLabels([]string{"cmd=a=b=c"}, true)
	if err != nil {
		t.Fatalf("parseLabels: %v", err)
	}
	if got["cmd"] != "a=b=c" {
		t.Errorf("cmd = %q, want %q", got["cmd"], "a=b=c")
	}
}

func TestParseLabelsMalformed(t *testing.T) {
	if _, err := parseLabels([]string{"app=ok", "broken"}, true); err == nil {
		t.Error("strict mode: want error for malformed label, got nil")
	}

	got, err := parseLabels([]string{"app=ok", "broken"}, false)
	if err != nil {
		t.Fatalf("lenient mode: %v", err)
	}
	if len(got) != 1 || got["app"] != "ok" {
		t.Errorf("lenient mode got %v, want only app=ok", got)
	}
}

func TestParseLabelsRejectsReservedInBothModes(t *testing.T) {
	for _, strict := range []bool{true, false} {
		_, err := parseLabels([]string{"app=ok", "io.oci-sync.encrypted=false"}, strict)
		if err == nil {
			t.Errorf("strict=%v: want error for reserved key, got nil", strict)
		} else if !strings.Contains(err.Error(), "reserved") {
			t.Errorf("strict=%v: error %q should mention reserved", strict, err)
		}
	}
}
