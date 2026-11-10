package jdm

import "testing"

func TestResolveField_MapStringBool(t *testing.T) {
	data := map[string]interface{}{
		"customer_condition": map[string]bool{
			"has_acne":    true,
			"has_stinger": false,
		},
	}

	if got := resolveField(data, "customer_condition.has_acne"); got != true {
		t.Errorf("has_acne: got %v, want true", got)
	}
	if got := resolveField(data, "customer_condition.has_stinger"); got != false {
		t.Errorf("has_stinger: got %v, want false", got)
	}
	if got := resolveField(data, "customer_condition.missing_flag"); got != nil {
		t.Errorf("missing_flag: got %v, want nil", got)
	}
}
