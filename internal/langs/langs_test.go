package langs

import "testing"

func TestValidateRequestLang(t *testing.T) {
	if err := ValidateRequestLang(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestLang("auto"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestLang("DE"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequestLang("xx"); err == nil {
		t.Fatal("expected error for xx")
	}
}

func TestValidateResponseLang(t *testing.T) {
	if err := ValidateResponseLang("ru"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResponseLang(""); err == nil {
		t.Fatal("expected empty error")
	}
	if err := ValidateResponseLang("de"); err == nil {
		t.Fatal("expected error for de response")
	}
}
