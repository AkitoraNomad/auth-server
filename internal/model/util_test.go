package model

import (
	"reflect"
	"testing"
)

func TestTextArrayRoundTrip(t *testing.T) {
	for _, input := range []TextArray{
		{}, {""}, {"profile", ""}, {"a,b", `a"b`, `a\b`, "NULL", " spaced ", "中文", "{braces}"},
	} {
		encoded, err := input.Value()
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []any{encoded, []byte(encoded.(string))} {
			var actual TextArray
			if err := actual.Scan(value); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(input, actual) {
				t.Fatalf("%#v became %#v via %q", input, actual, encoded)
			}
		}
	}
	var actual TextArray
	if err := actual.Scan(nil); err != nil || actual != nil {
		t.Fatalf("SQL NULL: %#v, %v", actual, err)
	}
	encoded, err := TextArray(nil).Value()
	if err != nil || encoded != "{}" {
		t.Fatalf("nil Value: %v, %v", encoded, err)
	}
}

func TestTextArrayRejectsInvalidValues(t *testing.T) {
	for _, value := range []any{"not-an-array", `{"unterminated}`, `{NULL}`, 42} {
		var actual TextArray
		if err := actual.Scan(value); err == nil {
			t.Fatalf("accepted %#v as %#v", value, actual)
		}
	}
}
