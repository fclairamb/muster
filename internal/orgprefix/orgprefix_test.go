package orgprefix

import (
	"reflect"
	"testing"
)

func TestDerive(t *testing.T) {
	cases := []struct {
		name      string
		orgs      []string
		overrides map[string]string
		want      map[string]string
	}{
		{
			name: "single",
			orgs: []string{"stonal-tech"},
			want: map[string]string{"stonal-tech": "s"},
		},
		{
			name: "two distinct first letters",
			orgs: []string{"stonal-tech", "fclairamb"},
			want: map[string]string{"stonal-tech": "s", "fclairamb": "f"},
		},
		{
			name: "collision expands both",
			orgs: []string{"stonal-tech", "some-org"},
			want: map[string]string{"stonal-tech": "st", "some-org": "so"},
		},
		{
			name:      "override frees other",
			orgs:      []string{"microsoft", "meta"},
			overrides: map[string]string{"microsoft": "ms"},
			want:      map[string]string{"microsoft": "ms", "meta": "m"},
		},
		{
			name: "three-way collision",
			orgs: []string{"microsoft", "meta", "mozilla"},
			want: map[string]string{"microsoft": "mi", "meta": "me", "mozilla": "mo"},
		},
		{
			name: "empty",
			orgs: nil,
			want: map[string]string{},
		},
		{
			name: "duplicate input deduped",
			orgs: []string{"foo", "foo"},
			want: map[string]string{"foo": "f"},
		},
		{
			name:      "override collides with non-fixed forces expansion",
			orgs:      []string{"meta", "microsoft"},
			overrides: map[string]string{"microsoft": "m"},
			want:      map[string]string{"microsoft": "m", "meta": "me"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Derive(tc.orgs, tc.overrides)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
