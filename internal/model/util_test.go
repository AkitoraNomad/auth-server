package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTextArrayJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input TextArray
		want  string
	}{
		{"nil", nil, `[]`},
		{"empty", TextArray{}, `[]`},
		{"populated", TextArray{"profile", "中文", `a"b`}, `["profile","中文","a\"b"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.input)
			if err != nil || string(got) != tc.want {
				t.Fatalf("JSON = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}

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
