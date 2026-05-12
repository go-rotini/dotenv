package dotenv

import "testing"

func TestPosition_String(t *testing.T) {
	cases := []struct {
		pos  Position
		want string
	}{
		{Position{}, "<unknown position>"},
		{Position{Line: 0, Column: 5}, "<unknown position>"},
		{Position{Line: 7}, "7"},
		{Position{Line: 7, Column: 3}, "7:3"},
		{Position{File: "x.env", Line: 7, Column: 3}, "x.env:7:3"},
		{Position{File: "x.env", Line: 7}, "x.env:7"},
	}
	for _, c := range cases {
		if got := c.pos.String(); got != c.want {
			t.Errorf("Position%+v.String() = %q, want %q", c.pos, got, c.want)
		}
	}
}
