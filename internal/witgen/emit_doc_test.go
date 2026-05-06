package witgen

import "testing"

func TestGoDocComment(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		indent string
		want   string
	}{
		{name: "empty", text: "", indent: "", want: ""},
		{
			name:   "single line",
			text:   "Hello world.",
			indent: "",
			want:   "// Hello world.\n",
		},
		{
			name:   "multi-line",
			text:   "First.\nSecond.",
			indent: "",
			want:   "// First.\n// Second.\n",
		},
		{
			name:   "blank line preserved",
			text:   "Para 1.\n\nPara 2.",
			indent: "",
			want:   "// Para 1.\n//\n// Para 2.\n",
		},
		{
			name:   "leading and trailing blanks stripped",
			text:   "\n\nBody.\n\n",
			indent: "",
			want:   "// Body.\n",
		},
		{
			name:   "three plus blank run collapses",
			text:   "A.\n\n\n\nB.",
			indent: "",
			want:   "// A.\n//\n// B.\n",
		},
		{
			name:   "indent applied",
			text:   "Field doc.",
			indent: "\t",
			want:   "\t// Field doc.\n",
		},
		{
			name:   "indent applied to blank line",
			text:   "Top.\n\nBottom.",
			indent: "\t",
			want:   "\t// Top.\n\t//\n\t// Bottom.\n",
		},
		{
			name:   "trailing space on text line stripped from blank handling only",
			text:   "Has  trailing.\n",
			indent: "",
			want:   "// Has  trailing.\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := goDocComment(c.text, c.indent)
			if got != c.want {
				t.Errorf("goDocComment(%q, %q):\n got %q\nwant %q", c.text, c.indent, got, c.want)
			}
		})
	}
}
