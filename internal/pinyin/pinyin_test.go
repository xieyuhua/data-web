package pinyin

import "testing"

func TestForms(t *testing.T) {
	full, ini := Forms("华东")
	if full != "huadong" || ini != "hd" {
		t.Fatalf("华东 -> %q/%q", full, ini)
	}
	full, ini = Forms("华南大区")
	if full != "huanandaqu" || ini != "hndq" {
		t.Fatalf("华南大区 -> %q/%q", full, ini)
	}
	// Latin text and digits are preserved.
	full, ini = Forms("HD01")
	if full != "hd01" || ini != "hd01" {
		t.Fatalf("HD01 -> %q/%q", full, ini)
	}
	if f, i := Forms("  "); f != "" || i != "" {
		t.Fatalf("blank -> %q/%q", f, i)
	}
}
